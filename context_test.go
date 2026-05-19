package gv8_test

import (
	"testing"

	"github.com/willyelm/gv8"
)

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
	for range 100 {
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
