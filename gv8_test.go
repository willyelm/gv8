package gv8_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/willyelm/gv8"
)

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
		IsModule:     true,
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

func TestCompileUnboundScript(t *testing.T) {
	iso := gv8.NewIsolate()
	defer iso.Dispose()

	ctx := gv8.NewContext(iso)
	defer ctx.Close()

	script, err := iso.CompileUnboundScript("40 + 2", gv8.ScriptOrigin{
		ResourceName: "test.js",
	})
	if err != nil {
		t.Fatalf("compile script: %v", err)
	}
	defer script.Release()

	value, err := script.Run(ctx)
	if err != nil {
		t.Fatalf("run script: %v", err)
	}
	defer value.Release()

	if got := value.Integer(); got != 42 {
		t.Fatalf("unexpected result: got %d want 42", got)
	}
}

func TestContextRunScript(t *testing.T) {
	iso := gv8.NewIsolate()
	defer iso.Dispose()

	ctx := gv8.NewContext(iso)
	defer ctx.Close()

	value, err := ctx.RunScript("6 * 7", "context-run-script.js")
	if err != nil {
		t.Fatalf("run script: %v", err)
	}
	defer value.Release()

	if got := value.Integer(); got != 42 {
		t.Fatalf("unexpected result: got %d want 42", got)
	}
}

func TestModuleNamespace(t *testing.T) {
	iso := gv8.NewIsolate()
	defer iso.Dispose()

	ctx := gv8.NewContext(iso)
	defer ctx.Close()

	mod, err := iso.CompileModule("export const answer = 42;", gv8.ScriptOrigin{
		ResourceName: "mod.js",
		IsModule:     true,
	})
	if err != nil {
		t.Fatalf("compile module: %v", err)
	}
	defer mod.Release()

	if err := mod.Instantiate(ctx, nil); err != nil {
		t.Fatalf("instantiate module: %v", err)
	}

	value, err := mod.Evaluate(ctx)
	if err != nil {
		t.Fatalf("evaluate module: %v", err)
	}
	if value != nil {
		defer value.Release()
	}

	ns := mod.Namespace(ctx)
	answer, err := ns.Get("answer")
	if err != nil {
		t.Fatalf("get export: %v", err)
	}
	defer answer.Release()

	if got := answer.Integer(); got != 42 {
		t.Fatalf("unexpected export: got %d want 42", got)
	}
}

func TestModuleResolver(t *testing.T) {
	iso := gv8.NewIsolate()
	defer iso.Dispose()

	ctx := gv8.NewContext(iso)
	defer ctx.Close()

	resolver := &testResolver{
		modules: map[string]string{
			"./dep.js": "export const answer = 41;",
		},
	}

	mod, err := iso.CompileModule(`
		import { answer } from "./dep.js";
		export const value = answer + 1;
	`, gv8.ScriptOrigin{
		ResourceName: "main.js",
		IsModule:     true,
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

	ns := mod.Namespace(ctx)
	exported, err := ns.Get("value")
	if err != nil {
		t.Fatalf("get export: %v", err)
	}
	defer exported.Release()

	if got := exported.Integer(); got != 42 {
		t.Fatalf("unexpected export: got %d want 42", got)
	}
}

func TestDynamicImport(t *testing.T) {
	iso := gv8.NewIsolate()
	defer iso.Dispose()

	ctx := gv8.NewContext(iso)
	defer ctx.Close()

	resolver := &testResolver{
		modules: map[string]string{
			"./dep.js": "export const answer = 41;",
		},
	}

	mod, err := iso.CompileModule(`
		export const value = await import("./dep.js").then((mod) => mod.answer + 1);
	`, gv8.ScriptOrigin{
		ResourceName: "main.js",
		IsModule:     true,
	})
	if err != nil {
		t.Fatalf("compile module: %v", err)
	}
	defer mod.Release()

	if got := mod.Status(); got != gv8.ModuleStatusUninstantiated {
		t.Fatalf("unexpected initial status: %v", got)
	}
	if mod.ScriptID() == 0 {
		t.Fatalf("expected non-zero script id")
	}

	if err := mod.Instantiate(ctx, resolver); err != nil {
		t.Fatalf("instantiate module: %v", err)
	}

	if got := mod.Status(); got != gv8.ModuleStatusInstantiated {
		t.Fatalf("unexpected instantiated status: %v", got)
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

	promise := value.Promise()
	defer promise.Release()
	for i := 0; i < 100 && promise.State() == gv8.PromisePending; i++ {
		iso.PerformMicrotaskCheckpoint()
	}

	if got := promise.State(); got != gv8.PromiseFulfilled {
		t.Fatalf("unexpected promise state: %v", got)
	}
	if got := mod.Status(); got != gv8.ModuleStatusEvaluated {
		t.Fatalf("unexpected evaluated status: %v", got)
	}

	exported, err := mod.Namespace(ctx).Get("value")
	if err != nil {
		t.Fatalf("get export: %v", err)
	}
	defer exported.Release()

	if got := exported.Integer(); got != 42 {
		t.Fatalf("unexpected export: got %d want 42", got)
	}
}

func TestModuleReadyNamespace(t *testing.T) {
	iso := gv8.NewIsolate()
	defer iso.Dispose()

	ctx := gv8.NewContext(iso)
	defer ctx.Close()

	resolver := &testResolver{
		modules: map[string]string{
			"./dep.js": "export const answer = 41;",
		},
	}

	mod, err := iso.CompileModule(`
		export const value = await import("./dep.js").then((mod) => mod.answer + 1);
	`, gv8.ScriptOrigin{
		ResourceName: "main.js",
		IsModule:     true,
	})
	if err != nil {
		t.Fatalf("compile module: %v", err)
	}
	defer mod.Release()

	namespace, err := mod.ReadyNamespace(ctx, resolver, nil)
	if err != nil {
		t.Fatalf("ready namespace: %v", err)
	}

	exported, err := namespace.Get("value")
	if err != nil {
		t.Fatalf("get export: %v", err)
	}
	defer exported.Release()

	if got := exported.Integer(); got != 42 {
		t.Fatalf("unexpected export: got %d want 42", got)
	}
}

func TestScriptRunAfterRelease(t *testing.T) {
	iso := gv8.NewIsolate()
	defer iso.Dispose()

	ctx := gv8.NewContext(iso)
	defer ctx.Close()

	script, err := iso.CompileUnboundScript("40 + 2", gv8.ScriptOrigin{
		ResourceName: "release.js",
	})
	if err != nil {
		t.Fatalf("compile script: %v", err)
	}

	script.Release()
	script.Release()

	if _, err := script.Run(ctx); err == nil {
		t.Fatalf("expected error running released script")
	}
}

func TestModuleRelease(t *testing.T) {
	iso := gv8.NewIsolate()
	defer iso.Dispose()

	mod, err := iso.CompileModule("export const answer = 42;", gv8.ScriptOrigin{
		ResourceName: "release-module.js",
		IsModule:     true,
	})
	if err != nil {
		t.Fatalf("compile module: %v", err)
	}

	mod.Release()
	mod.Release()

	if got := mod.Status(); got != gv8.ModuleStatusUninstantiated {
		t.Fatalf("unexpected status after release: %v", got)
	}
}

func TestContextCloseInvalidatesValues(t *testing.T) {
	iso := gv8.NewIsolate()
	defer iso.Dispose()

	ctx := gv8.NewContext(iso)

	value, err := ctx.RunScript(`({ answer: 42 })`, "close.js")
	if err != nil {
		t.Fatalf("run script: %v", err)
	}

	object := value.Object()
	ctx.Close()

	if got := value.Integer(); got != 0 {
		t.Fatalf("unexpected integer after close: %d", got)
	}

	if _, err := object.Get("answer"); err == nil {
		t.Fatalf("expected object get error after context close")
	}

	value.Release()
}

func TestContextClosePreventsNewWork(t *testing.T) {
	iso := gv8.NewIsolate()
	defer iso.Dispose()

	ctx := gv8.NewContext(iso)
	ctx.Close()

	if _, err := ctx.RunScript("40 + 2", "closed.js"); err == nil {
		t.Fatalf("expected run script error on closed context")
	}

	if _, err := ctx.NewObject(); err == nil {
		t.Fatalf("expected new object error on closed context")
	}
}

func TestContextCloseInvalidatesPromiseAndFunctionHandles(t *testing.T) {
	iso := gv8.NewIsolate()
	defer iso.Dispose()

	ctx := gv8.NewContext(iso)

	resolver, err := gv8.NewPromiseResolver(ctx)
	if err != nil {
		t.Fatalf("new promise resolver: %v", err)
	}

	fn, err := gv8.NewFunction(ctx, func(*gv8.FunctionCallbackInfo) (*gv8.Value, error) {
		return nil, nil
	})
	if err != nil {
		t.Fatalf("new function: %v", err)
	}

	promise := resolver.Promise()
	ctx.Close()

	if got := promise.State(); got != gv8.PromisePending {
		t.Fatalf("unexpected promise state after close: %v", got)
	}
	if _, err := fn.Call(nil); err == nil {
		t.Fatalf("expected function call error after context close")
	}
	if err := resolver.Resolve(fn.Value); err == nil {
		t.Fatalf("expected resolver resolve error after context close")
	}

	promise.Release()
	resolver.Release()
	fn.Release()
}

func TestRepeatedRunAndCloseCycle(t *testing.T) {
	for range 50 {
		iso := gv8.NewIsolate()
		ctx := gv8.NewContext(iso)

		value, err := ctx.RunScript("21 * 2", "loop.js")
		if err != nil {
			t.Fatalf("run script: %v", err)
		}
		if got := value.Integer(); got != 42 {
			t.Fatalf("unexpected result: got %d want 42", got)
		}

		ctx.Close()
		value.Release()
		iso.Dispose()
	}
}

func TestIntlNumberFormat(t *testing.T) {
	iso := gv8.NewIsolate()
	defer iso.Dispose()

	ctx := gv8.NewContext(iso)
	defer ctx.Close()

	script, err := iso.CompileUnboundScript(`
		new Intl.NumberFormat("en-US", {
			minimumFractionDigits: 2,
			maximumFractionDigits: 2,
		}).format(1234567.8)
	`, gv8.ScriptOrigin{
		ResourceName: "intl-number-format.js",
	})
	if err != nil {
		t.Fatalf("compile script: %v", err)
	}

	value, err := script.Run(ctx)
	if err != nil {
		t.Fatalf("run script: %v", err)
	}
	defer value.Release()

	if got := value.String(); got != "1,234,567.80" {
		t.Fatalf("unexpected Intl result: got %q want %q", got, "1,234,567.80")
	}
}

func TestTemporalPlainDate(t *testing.T) {
	iso := gv8.NewIsolate()
	defer iso.Dispose()

	ctx := gv8.NewContext(iso)
	defer ctx.Close()

	script, err := iso.CompileUnboundScript(`
		Temporal.PlainDate
			.from("2020-05-02")
			.add({ days: 1 })
			.toString()
	`, gv8.ScriptOrigin{
		ResourceName: "temporal-plain-date.js",
	})
	if err != nil {
		t.Fatalf("compile script: %v", err)
	}

	value, err := script.Run(ctx)
	if err != nil {
		t.Fatalf("run script: %v", err)
	}
	defer value.Release()

	if got := value.String(); got != "2020-05-03" {
		t.Fatalf("unexpected Temporal result: got %q want %q", got, "2020-05-03")
	}
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

func TestObjectSetGetAndCallMethod(t *testing.T) {
	iso := gv8.NewIsolate()
	defer iso.Dispose()

	ctx := gv8.NewContext(iso)
	defer ctx.Close()

	object, err := ctx.NewObject()
	if err != nil {
		t.Fatalf("new object: %v", err)
	}
	defer object.Release()

	if err := object.Set("message", "hello"); err != nil {
		t.Fatalf("set message: %v", err)
	}
	if err := object.Set("flag", true); err != nil {
		t.Fatalf("set flag: %v", err)
	}

	method, err := gv8.NewFunction(ctx, func(info *gv8.FunctionCallbackInfo) (*gv8.Value, error) {
		message, err := info.This().Get("message")
		if err != nil {
			return nil, err
		}
		defer message.Release()
		return gv8.NewStringValue(info.Context(), message.String()+" world")
	})
	if err != nil {
		t.Fatalf("new method: %v", err)
	}
	defer method.Release()

	if err := object.Set("speak", method); err != nil {
		t.Fatalf("set method: %v", err)
	}

	message, err := object.Get("message")
	if err != nil {
		t.Fatalf("get message: %v", err)
	}
	defer message.Release()
	if got := message.String(); got != "hello" {
		t.Fatalf("unexpected message: %q", got)
	}

	flag, err := object.Get("flag")
	if err != nil {
		t.Fatalf("get flag: %v", err)
	}
	defer flag.Release()
	if !flag.IsBoolean() || !flag.Boolean() {
		t.Fatalf("unexpected flag value")
	}

	result, err := object.CallMethod("speak")
	if err != nil {
		t.Fatalf("call method: %v", err)
	}
	defer result.Release()
	if got := result.String(); got != "hello world" {
		t.Fatalf("unexpected method result: %q", got)
	}
}

func TestPromiseResolver(t *testing.T) {
	iso := gv8.NewIsolate()
	defer iso.Dispose()

	ctx := gv8.NewContext(iso)
	defer ctx.Close()

	resolver, err := gv8.NewPromiseResolver(ctx)
	if err != nil {
		t.Fatalf("new promise resolver: %v", err)
	}
	defer resolver.Release()

	promise := resolver.Promise()
	defer promise.Release()

	if state := promise.State(); state != gv8.PromisePending {
		t.Fatalf("unexpected initial state: %v", state)
	}

	value, err := gv8.NewStringValue(ctx, "done")
	if err != nil {
		t.Fatalf("new string: %v", err)
	}
	defer value.Release()

	if err := resolver.Resolve(value); err != nil {
		t.Fatalf("resolve promise: %v", err)
	}
	iso.PerformMicrotaskCheckpoint()

	if state := promise.State(); state != gv8.PromiseFulfilled {
		t.Fatalf("unexpected fulfilled state: %v", state)
	}

	result := promise.Result()
	defer result.Release()
	if got := result.String(); got != "done" {
		t.Fatalf("unexpected promise result: %q", got)
	}
}

func TestPromiseAwait(t *testing.T) {
	iso := gv8.NewIsolate()
	defer iso.Dispose()

	ctx := gv8.NewContext(iso)
	defer ctx.Close()

	resolver, err := gv8.NewPromiseResolver(ctx)
	if err != nil {
		t.Fatalf("new promise resolver: %v", err)
	}
	defer resolver.Release()

	value, err := gv8.NewStringValue(ctx, "done")
	if err != nil {
		t.Fatalf("new string: %v", err)
	}
	defer value.Release()

	if err := resolver.Resolve(value); err != nil {
		t.Fatalf("resolve promise: %v", err)
	}

	result, err := resolver.Promise().Await(context.Background(), nil)
	if err != nil {
		t.Fatalf("await promise: %v", err)
	}
	defer result.Release()

	if got := result.String(); got != "done" {
		t.Fatalf("unexpected promise result: %q", got)
	}
}

func TestJSONHelpers(t *testing.T) {
	iso := gv8.NewIsolate()
	defer iso.Dispose()

	ctx := gv8.NewContext(iso)
	defer ctx.Close()

	value, err := gv8.JSONParse(ctx, `{"status":200,"ok":true,"message":"hello"}`)
	if err != nil {
		t.Fatalf("json parse: %v", err)
	}
	defer value.Release()

	if !value.IsObject() {
		t.Fatalf("expected object")
	}

	object := value.Object()
	status, err := object.Get("status")
	if err != nil {
		t.Fatalf("get status: %v", err)
	}
	defer status.Release()
	if got := status.Integer(); got != 200 {
		t.Fatalf("unexpected status: %d", got)
	}

	json, err := gv8.JSONStringify(ctx, value)
	if err != nil {
		t.Fatalf("json stringify: %v", err)
	}
	for _, want := range []string{`"status":200`, `"ok":true`, `"message":"hello"`} {
		if !strings.Contains(json, want) {
			t.Fatalf("stringify missing %s in %s", want, json)
		}
	}
}

func TestTypedArrayBytes(t *testing.T) {
	iso := gv8.NewIsolate()
	defer iso.Dispose()

	ctx := gv8.NewContext(iso)
	defer ctx.Close()

	script, err := iso.CompileUnboundScript(`
		const all = new Uint8Array([65, 66, 67, 68]);
		all.subarray(1, 3);
	`, gv8.ScriptOrigin{
		ResourceName: "typed-array.js",
	})
	if err != nil {
		t.Fatalf("compile script: %v", err)
	}

	value, err := script.Run(ctx)
	if err != nil {
		t.Fatalf("run script: %v", err)
	}
	defer value.Release()

	if !value.IsArrayBufferView() || !value.IsUint8Array() {
		t.Fatalf("expected Uint8Array view")
	}

	data, err := value.Bytes()
	if err != nil {
		t.Fatalf("value bytes: %v", err)
	}
	if got := string(data); got != "BC" {
		t.Fatalf("unexpected bytes: %q", got)
	}
}

func TestArrayBufferBytes(t *testing.T) {
	iso := gv8.NewIsolate()
	defer iso.Dispose()

	ctx := gv8.NewContext(iso)
	defer ctx.Close()

	script, err := iso.CompileUnboundScript(`
		const bytes = new Uint8Array([72, 73]);
		bytes.buffer;
	`, gv8.ScriptOrigin{
		ResourceName: "array-buffer.js",
	})
	if err != nil {
		t.Fatalf("compile script: %v", err)
	}

	value, err := script.Run(ctx)
	if err != nil {
		t.Fatalf("run script: %v", err)
	}
	defer value.Release()

	if !value.IsArrayBuffer() {
		t.Fatalf("expected ArrayBuffer")
	}

	data, err := value.Bytes()
	if err != nil {
		t.Fatalf("array buffer bytes: %v", err)
	}
	if got := string(data); got != "HI" {
		t.Fatalf("unexpected bytes: %q", got)
	}
}
