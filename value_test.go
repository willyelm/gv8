package gv8_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/willyelm/gv8"
)

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

	if got := mustString(t, value); got != "1,234,567.80" {
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

	if got := mustString(t, value); got != "2020-05-03" {
		t.Fatalf("unexpected Temporal result: got %q want %q", got, "2020-05-03")
	}
}

func TestWebAssemblyExecution(t *testing.T) {
	iso := gv8.NewIsolate()
	defer iso.Dispose()

	ctx := gv8.NewContext(iso)
	defer ctx.Close()

	value, err := ctx.RunScript(`
		const wasmBytes = new Uint8Array([
			0x00, 0x61, 0x73, 0x6d,
			0x01, 0x00, 0x00, 0x00,
			0x01, 0x05, 0x01, 0x60, 0x00, 0x01, 0x7f,
			0x03, 0x02, 0x01, 0x00,
			0x07, 0x0a, 0x01, 0x06, 0x61, 0x6e, 0x73, 0x77, 0x65, 0x72, 0x00, 0x00,
			0x0a, 0x06, 0x01, 0x04, 0x00, 0x41, 0x2a, 0x0b,
		]);

		const module = new WebAssembly.Module(wasmBytes);
		const instance = new WebAssembly.Instance(module);
		instance.exports.answer();
	`, "wasm.js")
	if err != nil {
		t.Fatalf("run script: %v", err)
	}
	defer value.Release()

	if got := value.Integer(); got != 42 {
		t.Fatalf("unexpected wasm result: got %d want 42", got)
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
		str, err := message.StringValue()
		if err != nil {
			return nil, err
		}
		return gv8.NewStringValue(info.Context(), str+" world")
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
	if got := mustString(t, message); got != "hello" {
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
	if got := mustString(t, result); got != "hello world" {
		t.Fatalf("unexpected method result: %q", got)
	}
}

func TestValueStringValueReturnsJSError(t *testing.T) {
	iso := gv8.NewIsolate()
	defer iso.Dispose()

	ctx := gv8.NewContext(iso)

	value, err := ctx.RunScript(`"ok"`, "string-value.js")
	if err != nil {
		t.Fatalf("run script: %v", err)
	}

	ctx.Close()

	if _, err := value.StringValue(); err == nil {
		t.Fatalf("expected string conversion error")
	} else {
		var jsErr *gv8.JSError
		if !errors.As(err, &jsErr) {
			t.Fatalf("expected JSError, got %T", err)
		}
		if extracted, ok := gv8.AsJSError(err); !ok || extracted == nil {
			t.Fatalf("expected AsJSError to extract JSError")
		}
		if !gv8.IsJSError(err) {
			t.Fatalf("expected IsJSError")
		}
	}

	value.Release()
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

func TestNewArrayBufferCopy(t *testing.T) {
	iso := gv8.NewIsolate()
	defer iso.Dispose()

	ctx := gv8.NewContext(iso)
	defer ctx.Close()

	source := []byte("gv8")
	value, err := gv8.NewArrayBufferCopy(ctx, source)
	if err != nil {
		t.Fatalf("new array buffer: %v", err)
	}
	defer value.Release()

	source[0] = 'x'
	if !value.IsArrayBuffer() || value.Len() != 3 {
		t.Fatalf("expected ArrayBuffer length 3")
	}
	data, err := value.Bytes()
	if err != nil {
		t.Fatalf("array buffer bytes: %v", err)
	}
	if got := string(data); got != "gv8" {
		t.Fatalf("unexpected bytes: %q", got)
	}
}

func TestNewUint8ArrayCopy(t *testing.T) {
	iso := gv8.NewIsolate()
	defer iso.Dispose()

	ctx := gv8.NewContext(iso)
	defer ctx.Close()

	value, err := gv8.NewUint8ArrayCopy(ctx, []byte{40, 2})
	if err != nil {
		t.Fatalf("new uint8 array: %v", err)
	}
	defer value.Release()

	if !value.IsUint8Array() || value.Len() != 2 {
		t.Fatalf("expected Uint8Array length 2")
	}

	global := ctx.Global()
	if err := global.Set("bytes", value); err != nil {
		t.Fatalf("set bytes: %v", err)
	}
	global.Release()

	result, err := ctx.RunScript(`bytes[0] + bytes[1]`, "uint8-array-copy.js")
	if err != nil {
		t.Fatalf("run script: %v", err)
	}
	defer result.Release()
	if got := result.Integer(); got != 42 {
		t.Fatalf("unexpected result: got %d want 42", got)
	}
}

func TestObjectIndexedPropertiesAndLength(t *testing.T) {
	iso := gv8.NewIsolate()
	defer iso.Dispose()

	ctx := gv8.NewContext(iso)
	defer ctx.Close()

	array, err := ctx.NewArray(2)
	if err != nil {
		t.Fatalf("new array: %v", err)
	}
	defer array.Release()

	if err := array.SetIdx(0, "go"); err != nil {
		t.Fatalf("set index 0: %v", err)
	}
	if err := array.SetIdx(1, "v8"); err != nil {
		t.Fatalf("set index 1: %v", err)
	}
	if array.Len() != 2 {
		t.Fatalf("unexpected length: got %d want 2", array.Len())
	}

	item, err := array.GetIdx(1)
	if err != nil {
		t.Fatalf("get index: %v", err)
	}
	defer item.Release()
	if got := mustString(t, item); got != "v8" {
		t.Fatalf("unexpected item: %q", got)
	}

	if !array.Has("length") || array.Has("missing") {
		t.Fatalf("unexpected property existence")
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
