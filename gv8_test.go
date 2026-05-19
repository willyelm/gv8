package gv8_test

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/willyelm/gv8"
)

// testResolver is a module resolver backed by an inline source map.
type testResolver struct {
	modules map[string]string
	cache   map[string]*gv8.Module
}

func (r *testResolver) ResolveModule(ctx *gv8.Context, specifier string, _ *gv8.Module) (*gv8.Module, error) {
	if r.cache == nil {
		r.cache = map[string]*gv8.Module{}
	}
	if mod, ok := r.cache[specifier]; ok {
		return mod, nil
	}

	source, ok := r.modules[specifier]
	if !ok {
		return nil, &gv8.JSError{Message: "module not found"}
	}

	mod, err := ctx.Isolate().CompileModule(source, gv8.ScriptOrigin{
		ResourceName: specifier,
	})
	if err != nil {
		return nil, err
	}

	r.cache[specifier] = mod
	return mod, nil
}

func (r *testResolver) ResolveDynamicImport(ctx *gv8.Context, specifier string, _ string) (*gv8.Promise, error) {
	resolver, err := gv8.NewPromiseResolver(ctx)
	if err != nil {
		return nil, err
	}

	mod, err := r.ResolveModule(ctx, specifier, nil)
	if err != nil {
		return nil, err
	}
	if mod.Status() == gv8.ModuleStatusUninstantiated {
		if err := mod.Instantiate(ctx, r); err != nil {
			return nil, err
		}
	}
	if mod.Status() == gv8.ModuleStatusInstantiated {
		value, err := mod.Evaluate(ctx)
		if err != nil {
			return nil, err
		}
		if value != nil {
			defer value.Release()
		}
	}
	if err := resolver.Resolve(mod.Namespace(ctx).Value); err != nil {
		return nil, err
	}
	return resolver.Promise(), nil
}

// isolatedResolver compiles a module with a fixed numeric export, used for
// resolver isolation tests.
type isolatedResolver struct {
	value int
	cache map[string]*gv8.Module
}

func (r *isolatedResolver) ResolveModule(ctx *gv8.Context, specifier string, _ *gv8.Module) (*gv8.Module, error) {
	if r.cache == nil {
		r.cache = map[string]*gv8.Module{}
	}
	if mod, ok := r.cache[specifier]; ok {
		return mod, nil
	}

	mod, err := ctx.Isolate().CompileModule(
		`export const answer = `+strconv.Itoa(r.value)+`;`,
		gv8.ScriptOrigin{ResourceName: specifier},
	)
	if err != nil {
		return nil, err
	}
	r.cache[specifier] = mod
	return mod, nil
}

func (r *isolatedResolver) ResolveDynamicImport(ctx *gv8.Context, specifier string, resourceName string) (*gv8.Promise, error) {
	base := testResolver{}
	base.modules = map[string]string{
		specifier: `export const answer = ` + strconv.Itoa(r.value) + `;`,
	}
	return base.ResolveDynamicImport(ctx, specifier, resourceName)
}

// mustBind creates a host function and sets it on the global object, failing
// the test on error.
func mustBind(t *testing.T, ctx *gv8.Context, name string, fn gv8.HostFunction) {
	t.Helper()
	f, err := gv8.NewFunction(ctx, fn)
	if err != nil {
		t.Fatalf("new function %s: %v", name, err)
	}
	t.Cleanup(func() { f.Release() })
	g := ctx.Global()
	defer g.Release()
	if err := g.Set(name, f); err != nil {
		t.Fatalf("set global %s: %v", name, err)
	}
}

// mustString calls StringValue on v, failing the test on error.
func mustString(t *testing.T, v *gv8.Value) string {
	t.Helper()
	s, err := v.StringValue()
	if err != nil {
		t.Fatalf("string value: %v", err)
	}
	return s
}

func TestHostFunctionAndObjectInterop(t *testing.T) {
	iso := gv8.NewIsolate()
	defer iso.Dispose()

	ctx := gv8.NewContext(iso)
	defer ctx.Close()

	global := ctx.Global()
	defer global.Release()

	add, err := gv8.NewFunction(ctx, func(info *gv8.FunctionCallbackInfo) (*gv8.Value, error) {
		args := info.Args()
		sum := args[0].Integer() + args[1].Integer()
		return gv8.NewIntegerValue(info.Context(), sum)
	})
	if err != nil {
		t.Fatalf("new function: %v", err)
	}
	defer add.Release()

	if err := global.Set("add", add); err != nil {
		t.Fatalf("set global add: %v", err)
	}

	script, err := iso.CompileUnboundScript("add(20, 22)", gv8.ScriptOrigin{
		ResourceName: "host-fn.js",
	})
	if err != nil {
		t.Fatalf("compile script: %v", err)
	}

	value, err := script.Run(ctx)
	if err != nil {
		t.Fatalf("run script: %v", err)
	}
	defer value.Release()

	if got := value.Integer(); got != 42 {
		t.Fatalf("unexpected result: got %d want 42", got)
	}
}

func TestHostFunctionThrowsJSError(t *testing.T) {
	iso := gv8.NewIsolate()
	defer iso.Dispose()

	ctx := gv8.NewContext(iso)
	defer ctx.Close()

	global := ctx.Global()
	defer global.Release()

	fail, err := gv8.NewFunction(ctx, func(info *gv8.FunctionCallbackInfo) (*gv8.Value, error) {
		return nil, errors.New("boom")
	})
	if err != nil {
		t.Fatalf("new function: %v", err)
	}
	defer fail.Release()

	if err := global.Set("fail", fail); err != nil {
		t.Fatalf("set fail: %v", err)
	}

	script, err := iso.CompileUnboundScript("fail()", gv8.ScriptOrigin{
		ResourceName: "host-error.js",
	})
	if err != nil {
		t.Fatalf("compile script: %v", err)
	}

	if _, err := script.Run(ctx); err == nil {
		t.Fatalf("expected error")
	} else {
		var jsErr *gv8.JSError
		if !errors.As(err, &jsErr) {
			t.Fatalf("expected JSError, got %T", err)
		}
		if jsErr.Message != "Error: boom" {
			t.Fatalf("unexpected message: %q", jsErr.Message)
		}
	}
}

func TestHostCallbackCanReenterIsolateOnSameThread(t *testing.T) {
	iso := gv8.NewIsolate()
	defer iso.Dispose()

	ctx := gv8.NewContext(iso)
	defer ctx.Close()

	mustBind(t, ctx, "reenter", func(info *gv8.FunctionCallbackInfo) (*gv8.Value, error) {
		value, err := info.Context().RunScript("21 * 2", "reenter.js")
		if err != nil {
			return nil, err
		}
		return value, nil
	})

	value, err := ctx.RunScript("reenter()", "reenter-host.js")
	if err != nil {
		t.Fatalf("run script: %v", err)
	}
	defer value.Release()

	if got := value.Integer(); got != 42 {
		t.Fatalf("unexpected result: got %d want 42", got)
	}
}

func TestServerRuntimeEssentials(t *testing.T) {
	iso := gv8.NewIsolate()
	defer iso.Dispose()

	ctx := gv8.NewContext(iso)
	defer ctx.Close()

	hostCalls := 0
	mustBind(t, ctx, "hostAdd", func(info *gv8.FunctionCallbackInfo) (*gv8.Value, error) {
		hostCalls++
		args := info.Args()
		return gv8.NewIntegerValue(info.Context(), args[0].Integer()+args[1].Integer())
	})

	resolver := &testResolver{
		modules: map[string]string{
			"./static.js":  `export const base = hostAdd(20, 1);`,
			"./dynamic.js": `export const extra = 21;`,
		},
	}

	mod, err := iso.CompileModule(`
		import { base } from "./static.js";

		const dynamic = await import("./dynamic.js");
		const payload = JSON.parse('{"tag":"server-runtime","ok":true}');
		const bytes = new Uint8Array([base, dynamic.extra]);

		export const result = {
			total: base + dynamic.extra,
			tag: payload.tag,
			bytes,
		};
	`, gv8.ScriptOrigin{
		ResourceName: "server-runtime.js",
	})
	if err != nil {
		t.Fatalf("compile module: %v", err)
	}
	defer mod.Release()

	if err := mod.Instantiate(ctx, resolver); err != nil {
		t.Fatalf("instantiate module: %v", err)
	}

	value, err := mod.Evaluate(ctx)
	if err != nil {
		t.Fatalf("evaluate module: %v", err)
	}
	if value != nil {
		defer value.Release()
	}
	if value == nil || !value.IsPromise() {
		t.Fatalf("expected top-level await promise")
	}

	modulePromise := value.Promise()
	defer modulePromise.Release()
	for i := 0; i < 100 && modulePromise.State() == gv8.PromisePending; i++ {
		iso.PerformMicrotaskCheckpoint()
	}

	if hostCalls != 1 {
		t.Fatalf("unexpected host callback count: got %d want 1", hostCalls)
	}
	if got := modulePromise.State(); got != gv8.PromiseFulfilled {
		t.Fatalf("unexpected module promise state: %v", got)
	}

	result, err := mod.Namespace(ctx).Get("result")
	if err != nil {
		t.Fatalf("get result export: %v", err)
	}
	defer result.Release()

	resultObject := result.Object()
	total, err := resultObject.Get("total")
	if err != nil {
		t.Fatalf("get total: %v", err)
	}
	defer total.Release()
	if got := total.Integer(); got != 42 {
		t.Fatalf("unexpected total: got %d want 42", got)
	}

	tag, err := resultObject.Get("tag")
	if err != nil {
		t.Fatalf("get tag: %v", err)
	}
	defer tag.Release()
	if got := mustString(t, tag); got != "server-runtime" {
		t.Fatalf("unexpected tag: %q", got)
	}

	bytesValue, err := resultObject.Get("bytes")
	if err != nil {
		t.Fatalf("get bytes: %v", err)
	}
	defer bytesValue.Release()

	bytes, err := bytesValue.Bytes()
	if err != nil {
		t.Fatalf("bytes: %v", err)
	}
	if len(bytes) != 2 || bytes[0] != 21 || bytes[1] != 21 {
		t.Fatalf("unexpected bytes: %#v", bytes)
	}

	jsonValue, err := gv8.JSONParse(ctx, `{"mode":"server-runtime","count":42}`)
	if err != nil {
		t.Fatalf("json parse: %v", err)
	}
	defer jsonValue.Release()

	jsonText, err := gv8.JSONStringify(ctx, jsonValue)
	if err != nil {
		t.Fatalf("json stringify: %v", err)
	}
	for _, want := range []string{`"mode":"server-runtime"`, `"count":42`} {
		if !strings.Contains(jsonText, want) {
			t.Fatalf("stringify missing %s in %s", want, jsonText)
		}
	}

	resolverPromise, err := gv8.NewPromiseResolver(ctx)
	if err != nil {
		t.Fatalf("new promise resolver: %v", err)
	}
	defer resolverPromise.Release()

	doneValue, err := gv8.NewStringValue(ctx, "done")
	if err != nil {
		t.Fatalf("new string: %v", err)
	}
	defer doneValue.Release()

	pumpCalls := 0
	awaited, err := resolverPromise.Promise().Await(context.Background(), func(context.Context) error {
		pumpCalls++
		if pumpCalls == 2 {
			return resolverPromise.Resolve(doneValue)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("await resolver promise: %v", err)
	}
	defer awaited.Release()

	if pumpCalls != 2 {
		t.Fatalf("unexpected await pump count: got %d want 2", pumpCalls)
	}
	if got := mustString(t, awaited); got != "done" {
		t.Fatalf("unexpected awaited value: %q", got)
	}
}
