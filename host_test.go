package gv8

import (
	"context"
	"testing"
)

func hostFunctionCount() int {
	hostFnMu.RLock()
	defer hostFnMu.RUnlock()
	return len(hostFnRefs)
}

func contextCount() int {
	ctxMu.RLock()
	defer ctxMu.RUnlock()
	return len(ctxRefs)
}

func liveValueCount(ctx *Context) int {
	ctx.mu.RLock()
	defer ctx.mu.RUnlock()
	return len(ctx.liveValues)
}

func TestContextCloseReleasesHostFunctions(t *testing.T) {
	before := hostFunctionCount()

	iso := NewIsolate()
	defer iso.Dispose()

	ctx := NewContext(iso)
	if _, err := NewFunction(ctx, func(*FunctionCallbackInfo) (*Value, error) {
		return nil, nil
	}); err != nil {
		t.Fatalf("new function: %v", err)
	}
	if _, err := NewFunction(ctx, func(*FunctionCallbackInfo) (*Value, error) {
		return nil, nil
	}); err != nil {
		t.Fatalf("new function: %v", err)
	}

	if got := hostFunctionCount(); got != before+2 {
		t.Fatalf("unexpected host function count before close: got %d want %d", got, before+2)
	}

	ctx.Close()

	if got := hostFunctionCount(); got != before {
		t.Fatalf("unexpected host function count after close: got %d want %d", got, before)
	}
}

func TestContextCloseClearsRegistryAndResolver(t *testing.T) {
	before := contextCount()

	iso := NewIsolate()
	defer iso.Dispose()

	ctx := NewContext(iso)
	ctx.moduleResolversByScript[1] = testModuleResolver{}
	ctx.moduleResolversByName["main.js"] = testModuleResolver{}

	if got := contextCount(); got != before+1 {
		t.Fatalf("unexpected context count before close: got %d want %d", got, before+1)
	}

	ctx.Close()

	if got := contextCount(); got != before {
		t.Fatalf("unexpected context count after close: got %d want %d", got, before)
	}
	if ctx.moduleResolversByScript != nil {
		t.Fatalf("expected script resolver bindings to be cleared on close")
	}
	if ctx.moduleResolversByName != nil {
		t.Fatalf("expected resource resolver bindings to be cleared on close")
	}
}

func TestRepeatedContextLifecycleDoesNotLeakRegistrations(t *testing.T) {
	beforeHostFns := hostFunctionCount()
	beforeContexts := contextCount()

	for range 50 {
		iso := NewIsolate()
		ctx := NewContext(iso)

		if _, err := NewFunction(ctx, func(*FunctionCallbackInfo) (*Value, error) {
			return nil, nil
		}); err != nil {
			t.Fatalf("new function: %v", err)
		}
		if _, err := NewFunction(ctx, func(*FunctionCallbackInfo) (*Value, error) {
			return nil, nil
		}); err != nil {
			t.Fatalf("new function: %v", err)
		}

		ctx.Close()
		iso.Dispose()
	}

	if got := hostFunctionCount(); got != beforeHostFns {
		t.Fatalf("unexpected host function count after lifecycle loop: got %d want %d", got, beforeHostFns)
	}
	if got := contextCount(); got != beforeContexts {
		t.Fatalf("unexpected context count after lifecycle loop: got %d want %d", got, beforeContexts)
	}
}

func TestHostFunctionInvocationDoesNotLeakTrackedValues(t *testing.T) {
	iso := NewIsolate()
	defer iso.Dispose()

	ctx := NewContext(iso)
	defer ctx.Close()

	fn, err := NewFunction(ctx, func(info *FunctionCallbackInfo) (*Value, error) {
		args := info.Args()
		if len(args) > 0 {
			_ = args[0].Integer()
		}
		return NewIntegerValue(info.Context(), 42)
	})
	if err != nil {
		t.Fatalf("new function: %v", err)
	}
	defer fn.Release()

	global := ctx.Global()
	if err := global.Set("answer", fn); err != nil {
		t.Fatalf("set global: %v", err)
	}
	global.Release()

	before := liveValueCount(ctx)
	for range 100 {
		value, err := ctx.RunScript("answer(1)", "callback-leak.js")
		if err != nil {
			t.Fatalf("run script: %v", err)
		}
		value.Release()
	}

	if got := liveValueCount(ctx); got != before {
		t.Fatalf("unexpected live value count after callback loop: got %d want %d", got, before)
	}
}

func TestHostFunctionCanReturnCallbackArgument(t *testing.T) {
	iso := NewIsolate()
	defer iso.Dispose()

	ctx := NewContext(iso)
	defer ctx.Close()

	fn, err := NewFunction(ctx, func(info *FunctionCallbackInfo) (*Value, error) {
		return info.Args()[0], nil
	})
	if err != nil {
		t.Fatalf("new function: %v", err)
	}
	defer fn.Release()

	global := ctx.Global()
	if err := global.Set("identity", fn); err != nil {
		t.Fatalf("set global: %v", err)
	}
	global.Release()

	value, err := ctx.RunScript(`identity("ok")`, "callback-arg.js")
	if err != nil {
		t.Fatalf("run script: %v", err)
	}
	defer value.Release()

	if got := value.String(); got != "ok" {
		t.Fatalf("unexpected callback result: %q", got)
	}
}

func TestMixedLifecycleStressDoesNotLeakOrBreakTeardown(t *testing.T) {
	beforeHostFns := hostFunctionCount()
	beforeContexts := contextCount()

	for range 200 {
		iso := NewIsolate()
		ctx := NewContext(iso)

		fn, err := NewFunction(ctx, func(info *FunctionCallbackInfo) (*Value, error) {
			return NewStringValue(info.Context(), info.Args()[0].String())
		})
		if err != nil {
			t.Fatalf("new function: %v", err)
		}

		global := ctx.Global()
		if err := global.Set("echo", fn); err != nil {
			t.Fatalf("set global: %v", err)
		}
		global.Release()

		scriptValue, err := ctx.RunScript(`echo("ok")`, "stress.js")
		if err != nil {
			t.Fatalf("run script: %v", err)
		}
		if got := scriptValue.String(); got != "ok" {
			t.Fatalf("unexpected script value: %q", got)
		}
		scriptValue.Release()

		mod, err := iso.CompileModule(`
			export const value = await import("./dep.js").then((mod) => mod.answer + 1);
		`, ScriptOrigin{ResourceName: "stress-mod.js", IsModule: true})
		if err != nil {
			t.Fatalf("compile module: %v", err)
		}

		resolver := &stressResolver{
			modules: map[string]string{"./dep.js": `export const answer = 41;`},
		}
		if err := mod.Instantiate(ctx, resolver); err != nil {
			t.Fatalf("instantiate module: %v", err)
		}
		namespace, err := mod.ReadyNamespace(ctx, resolver, nil)
		if err != nil {
			t.Fatalf("ready namespace: %v", err)
		}
		value, err := namespace.Get("value")
		if err != nil {
			t.Fatalf("get export: %v", err)
		}
		if got := value.Integer(); got != 42 {
			t.Fatalf("unexpected export: got %d want 42", got)
		}
		value.Release()
		mod.Release()

		resolverPromise, err := NewPromiseResolver(ctx)
		if err != nil {
			t.Fatalf("new promise resolver: %v", err)
		}
		promiseValue, err := NewStringValue(ctx, "done")
		if err != nil {
			t.Fatalf("new string: %v", err)
		}
		if err := resolverPromise.Resolve(promiseValue); err != nil {
			t.Fatalf("resolve promise: %v", err)
		}
		awaited, err := resolverPromise.Promise().Await(context.Background(), nil)
		if err != nil {
			t.Fatalf("await promise: %v", err)
		}
		if got := awaited.String(); got != "done" {
			t.Fatalf("unexpected awaited value: %q", got)
		}
		awaited.Release()
		promiseValue.Release()
		resolverPromise.Release()
		fn.Release()

		ctx.Close()
		iso.Dispose()
	}

	if got := hostFunctionCount(); got != beforeHostFns {
		t.Fatalf("unexpected host function count after stress loop: got %d want %d", got, beforeHostFns)
	}
	if got := contextCount(); got != beforeContexts {
		t.Fatalf("unexpected context count after stress loop: got %d want %d", got, beforeContexts)
	}
}

type testModuleResolver struct{}

func (testModuleResolver) ResolveModule(*Context, string, *Module) (*Module, error) {
	return nil, nil
}

type stressResolver struct {
	modules map[string]string
}

func (r *stressResolver) ResolveModule(ctx *Context, specifier string, _ *Module) (*Module, error) {
	source, ok := r.modules[specifier]
	if !ok {
		return nil, &JSError{Message: "module not found"}
	}
	return ctx.Isolate().CompileModule(source, ScriptOrigin{
		ResourceName: specifier,
		IsModule:     true,
	})
}

func (r *stressResolver) ResolveDynamicImport(ctx *Context, specifier string, _ string) (*Promise, error) {
	resolver, err := NewPromiseResolver(ctx)
	if err != nil {
		return nil, err
	}
	mod, err := r.ResolveModule(ctx, specifier, nil)
	if err != nil {
		return nil, err
	}
	if err := mod.Instantiate(ctx, r); err != nil {
		return nil, err
	}
	if value, err := mod.Evaluate(ctx); err == nil && value != nil {
		defer value.Release()
	}
	if err := resolver.Resolve(mod.Namespace(ctx).Value); err != nil {
		return nil, err
	}
	return resolver.Promise(), nil
}
