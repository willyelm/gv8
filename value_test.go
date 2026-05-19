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
