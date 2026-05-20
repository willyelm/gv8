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

func TestIsolateFromSnapshot(t *testing.T) {
	builder := gv8.NewSnapshotBuilder()
	defer builder.Release()

	if err := builder.RunScript(`globalThis.answer = 42`, gv8.ScriptOrigin{
		ResourceName: "snapshot.js",
	}); err != nil {
		t.Fatalf("snapshot run script: %v", err)
	}
	snapshot, err := builder.Build()
	if err != nil {
		t.Fatalf("snapshot build: %v", err)
	}
	if len(snapshot) == 0 {
		t.Fatalf("expected snapshot data")
	}

	iso := gv8.NewIsolateWithOptions(gv8.IsolateOptions{Snapshot: snapshot})
	defer iso.Dispose()

	ctx := gv8.NewContext(iso)
	defer ctx.Close()

	value, err := ctx.RunScript(`answer`, "from-snapshot.js")
	if err != nil {
		t.Fatalf("run script: %v", err)
	}
	defer value.Release()
	if got := value.Integer(); got != 42 {
		t.Fatalf("unexpected snapshot value: got %d want 42", got)
	}
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
		if info.Error.Stack == "" {
			t.Fatalf("expected stack trace in rejection info for production diagnosis")
		}
		if !strings.Contains(info.Error.Stack, "promise-reject.js") {
			t.Fatalf("expected rejection stack to contain script name, got: %q", info.Error.Stack)
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
		if !strings.Contains(jsErr.Location, "exception.js") {
			t.Fatalf("expected location to contain script name, got: %q", jsErr.Location)
		}
		if jsErr.Stack == "" {
			t.Fatalf("expected non-empty stack trace for production diagnosis")
		}
		if !strings.Contains(jsErr.Stack, "exception.js") {
			t.Fatalf("expected stack to contain script name, got: %q", jsErr.Stack)
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

func TestIsolateExceptionStackTraceContainsCallFrames(t *testing.T) {
	iso := gv8.NewIsolate()
	defer iso.Dispose()

	ctx := gv8.NewContext(iso)
	defer ctx.Close()

	errorsCh := make(chan *gv8.JSError, 1)
	iso.SetExceptionHandler(func(err *gv8.JSError) {
		select {
		case errorsCh <- err:
		default:
		}
	})

	if _, err := ctx.RunScript(`
function inner() { throw new Error("frame-test") }
function outer() { inner() }
outer()
`, "frames.js"); err == nil {
		t.Fatalf("expected script error")
	}

	select {
	case jsErr := <-errorsCh:
		if jsErr == nil {
			t.Fatalf("expected JSError from exception hook")
		}
		if !strings.Contains(jsErr.Stack, "inner") {
			t.Fatalf("expected stack to contain 'inner' for call frame diagnosis, got: %q", jsErr.Stack)
		}
		if !strings.Contains(jsErr.Stack, "outer") {
			t.Fatalf("expected stack to contain 'outer' for call frame diagnosis, got: %q", jsErr.Stack)
		}
		if !strings.Contains(jsErr.Stack, "frames.js") {
			t.Fatalf("expected stack to contain script name, got: %q", jsErr.Stack)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for exception hook")
	}
}

func TestObservabilitySnapshotCountsAreExact(t *testing.T) {
	iso := gv8.NewIsolate()
	defer iso.Dispose()

	ctx := gv8.NewContext(iso)
	defer ctx.Close()

	baseline := iso.ObservabilitySnapshot()

	for range 3 {
		_, _ = ctx.RunScript(`throw new Error("count")`, "count.js")
	}
	if got := iso.ObservabilitySnapshot().Exceptions - baseline.Exceptions; got != 3 {
		t.Fatalf("unexpected exception count: got %d want 3", got)
	}

	for range 2 {
		_, _ = ctx.RunScript(`Promise.reject(new Error("rej"))`, "rej.js")
	}
	iso.PerformMicrotaskCheckpoint()
	if got := iso.ObservabilitySnapshot().UnhandledPromiseRejections - baseline.UnhandledPromiseRejections; got != 2 {
		t.Fatalf("unexpected rejection count: got %d want 2", got)
	}

	mod, err := iso.CompileModule(
		`import { x } from "./missing.js"; export const y = x;`,
		gv8.ScriptOrigin{ResourceName: "count-mod.js"},
	)
	if err != nil {
		t.Fatalf("compile module: %v", err)
	}
	defer mod.Release()
	_ = mod.Instantiate(ctx, nil)
	if got := iso.ObservabilitySnapshot().ModuleResolutionFailures - baseline.ModuleResolutionFailures; got != 1 {
		t.Fatalf("unexpected module failure count: got %d want 1", got)
	}
}

func TestHeapStatisticsReflectsIsolateState(t *testing.T) {
	iso := gv8.NewIsolateWithOptions(gv8.IsolateOptions{
		InitialHeapSizeBytes: 8 << 20,
		MaxHeapSizeBytes:     32 << 20,
	})
	defer iso.Dispose()

	ctx := gv8.NewContext(iso)
	defer ctx.Close()

	if _, err := ctx.RunScript(`
		const arr = new Array(1000).fill(null).map((_, i) => ({ index: i }));
		arr.length;
	`, "heap-test.js"); err != nil {
		t.Fatalf("run script: %v", err)
	}

	stats := iso.HeapStatistics()
	if stats.UsedHeapSize == 0 {
		t.Fatalf("expected non-zero used heap size")
	}
	if stats.TotalHeapSize == 0 {
		t.Fatalf("expected non-zero total heap size")
	}
	if stats.HeapSizeLimit == 0 {
		t.Fatalf("expected non-zero heap size limit")
	}
	if stats.UsedHeapSize > stats.HeapSizeLimit {
		t.Fatalf("used heap %d exceeds limit %d", stats.UsedHeapSize, stats.HeapSizeLimit)
	}
}
