package gv8

import "testing"

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
	ctx.moduleResolver = testModuleResolver{}

	if got := contextCount(); got != before+1 {
		t.Fatalf("unexpected context count before close: got %d want %d", got, before+1)
	}

	ctx.Close()

	if got := contextCount(); got != before {
		t.Fatalf("unexpected context count after close: got %d want %d", got, before)
	}
	if ctx.moduleResolver != nil {
		t.Fatalf("expected module resolver to be cleared on close")
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

type testModuleResolver struct{}

func (testModuleResolver) ResolveModule(*Context, string, *Module) (*Module, error) {
	return nil, nil
}
