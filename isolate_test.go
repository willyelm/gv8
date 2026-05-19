package gv8_test

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/willyelm/gv8"
)

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

func TestConcurrentAccessRequiresExternalSynchronization(t *testing.T) {
	prev := runtime.GOMAXPROCS(2)
	defer runtime.GOMAXPROCS(prev)

	iso := gv8.NewIsolate()
	defer iso.Dispose()

	ctx := gv8.NewContext(iso)
	defer ctx.Close()

	entered := make(chan struct{})
	release := make(chan struct{})

	mustBind(t, ctx, "block", func(*gv8.FunctionCallbackInfo) (*gv8.Value, error) {
		close(entered)
		<-release
		return nil, nil
	})

	done := make(chan error, 1)
	go func() {
		_, err := ctx.RunScript("block()", "block.js")
		done <- err
	}()

	<-entered

	if _, err := ctx.RunScript("40 + 2", "concurrent.js"); err == nil {
		t.Fatalf("expected concurrent access error")
	} else if !strings.Contains(err.Error(), "externally synchronize access") {
		t.Fatalf("unexpected concurrent access error: %v", err)
	}

	close(release)

	if err := <-done; err != nil {
		t.Fatalf("unexpected blocking script error: %v", err)
	}
}

func TestConcurrentObjectAccessRequiresExternalSynchronization(t *testing.T) {
	prev := runtime.GOMAXPROCS(2)
	defer runtime.GOMAXPROCS(prev)

	iso := gv8.NewIsolate()
	defer iso.Dispose()

	ctx := gv8.NewContext(iso)
	defer ctx.Close()

	entered := make(chan struct{})
	release := make(chan struct{})

	mustBind(t, ctx, "block", func(*gv8.FunctionCallbackInfo) (*gv8.Value, error) {
		close(entered)
		<-release
		return nil, nil
	})

	object, err := ctx.NewObject()
	if err != nil {
		t.Fatalf("new object: %v", err)
	}
	defer object.Release()

	if err := object.Set("answer", 42); err != nil {
		t.Fatalf("set answer: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := ctx.RunScript("block()", "block-object.js")
		done <- err
	}()

	<-entered

	if _, err := object.Get("answer"); err == nil {
		t.Fatalf("expected concurrent object access error")
	} else if !strings.Contains(err.Error(), "externally synchronize access") {
		t.Fatalf("unexpected concurrent object access error: %v", err)
	}

	close(release)

	if err := <-done; err != nil {
		t.Fatalf("unexpected blocking script error: %v", err)
	}
}

func TestIsolateTerminateExecution(t *testing.T) {
	prev := runtime.GOMAXPROCS(2)
	defer runtime.GOMAXPROCS(prev)

	iso := gv8.NewIsolate()
	defer iso.Dispose()

	ctx := gv8.NewContext(iso)
	defer ctx.Close()

	started := make(chan struct{}, 1)
	mustBind(t, ctx, "tick", func(*gv8.FunctionCallbackInfo) (*gv8.Value, error) {
		select {
		case started <- struct{}{}:
		default:
		}
		return nil, nil
	})

	done := make(chan error, 1)
	go func() {
		_, err := ctx.RunScript(`while (true) { tick() }`, "terminate.js")
		done <- err
	}()

	<-started
	iso.TerminateExecution()

	err := <-done
	if err == nil {
		t.Fatalf("expected termination error")
	}

	iso.CancelTerminateExecution()

	value, err := ctx.RunScript("40 + 2", "after-terminate.js")
	if err != nil {
		t.Fatalf("run script after cancel terminate: %v", err)
	}
	defer value.Release()

	if got := value.Integer(); got != 42 {
		t.Fatalf("unexpected result after cancel terminate: got %d want 42", got)
	}
}

func TestIsolateTerminateOnContextDone(t *testing.T) {
	prev := runtime.GOMAXPROCS(2)
	defer runtime.GOMAXPROCS(prev)

	iso := gv8.NewIsolate()
	defer iso.Dispose()

	ctx := gv8.NewContext(iso)
	defer ctx.Close()

	started := make(chan struct{}, 1)
	mustBind(t, ctx, "tick", func(*gv8.FunctionCallbackInfo) (*gv8.Value, error) {
		select {
		case started <- struct{}{}:
		default:
		}
		return nil, nil
	})

	waitCtx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	stopWatchdog := iso.TerminateOnContextDone(waitCtx)
	defer stopWatchdog()

	done := make(chan error, 1)
	go func() {
		_, err := ctx.RunScript(`while (true) { tick() }`, "watchdog.js")
		done <- err
	}()

	<-started

	err := <-done
	if err == nil {
		t.Fatalf("expected watchdog termination error")
	}

	iso.CancelTerminateExecution()
}

func TestIsolateHeapStatisticsAndMemoryPressureControls(t *testing.T) {
	iso := gv8.NewIsolateWithOptions(gv8.IsolateOptions{
		InitialHeapSizeBytes: 8 << 20,
		MaxHeapSizeBytes:     32 << 20,
	})
	defer iso.Dispose()

	stats := iso.HeapStatistics()
	if stats.HeapSizeLimit == 0 {
		t.Fatalf("expected non-zero heap size limit")
	}

	iso.LowMemoryNotification()
	iso.MemoryPressureNotification(gv8.MemoryPressureModerate)
	iso.MemoryPressureNotification(gv8.MemoryPressureCritical)
}

func TestIsolateUnhandledPromiseRejectionObservability(t *testing.T) {
	iso := gv8.NewIsolate()
	defer iso.Dispose()

	ctx := gv8.NewContext(iso)
	defer ctx.Close()

	events := make(chan gv8.PromiseRejectInfo, 4)
	iso.SetUnhandledPromiseRejectionHandler(func(info gv8.PromiseRejectInfo) {
		events <- info
	})

	if _, err := ctx.RunScript(`Promise.reject(new Error("boom"))`, "promise-reject.js"); err != nil {
		t.Fatalf("run script: %v", err)
	}
	iso.PerformMicrotaskCheckpoint()

	select {
	case info := <-events:
		if info.Event != gv8.PromiseRejectWithNoHandler {
			t.Fatalf("unexpected promise reject event: %v", info.Event)
		}
		if info.Error == nil || !strings.Contains(info.Error.Message, "Error: boom") {
			t.Fatalf("unexpected promise reject error: %#v", info.Error)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for promise rejection hook")
	}

	snapshot := iso.ObservabilitySnapshot()
	if snapshot.UnhandledPromiseRejections == 0 {
		t.Fatalf("expected unhandled promise rejection count to increase")
	}
}

func TestIsolateExceptionObservability(t *testing.T) {
	iso := gv8.NewIsolate()
	defer iso.Dispose()

	ctx := gv8.NewContext(iso)
	defer ctx.Close()

	errorsCh := make(chan *gv8.JSError, 2)
	iso.SetExceptionHandler(func(err *gv8.JSError) {
		errorsCh <- err
	})

	if _, err := ctx.RunScript(`throw new Error("boom")`, "exception.js"); err == nil {
		t.Fatalf("expected script error")
	}

	select {
	case jsErr := <-errorsCh:
		if jsErr == nil || !strings.Contains(jsErr.Message, "Error: boom") {
			t.Fatalf("unexpected exception hook value: %#v", jsErr)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for exception hook")
	}

	snapshot := iso.ObservabilitySnapshot()
	if snapshot.Exceptions == 0 {
		t.Fatalf("expected exception count to increase")
	}
}

func TestIsolateModuleResolutionFailureObservability(t *testing.T) {
	iso := gv8.NewIsolate()
	defer iso.Dispose()

	ctx := gv8.NewContext(iso)
	defer ctx.Close()

	failures := make(chan string, 2)
	iso.SetModuleResolutionFailureHandler(func(specifier string, referrer string, err error) {
		failures <- specifier + "|" + referrer + "|" + err.Error()
	})

	mod, err := iso.CompileModule(`
		import { answer } from "./dep.js";
		export const value = answer + 1;
	`, gv8.ScriptOrigin{ResourceName: "missing.js"})
	if err != nil {
		t.Fatalf("compile module: %v", err)
	}
	defer mod.Release()

	if err := mod.Instantiate(ctx, nil); err == nil {
		t.Fatalf("expected instantiate error")
	}

	select {
	case failure := <-failures:
		if !strings.Contains(failure, "./dep.js") {
			t.Fatalf("unexpected failure hook value: %q", failure)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for module failure hook")
	}

	snapshot := iso.ObservabilitySnapshot()
	if snapshot.ModuleResolutionFailures == 0 {
		t.Fatalf("expected module resolution failure count to increase")
	}
}
