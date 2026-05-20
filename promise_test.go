package gv8_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/willyelm/gv8"
)

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
	if got := mustString(t, result); got != "done" {
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

	if got := mustString(t, result); got != "done" {
		t.Fatalf("unexpected promise result: %q", got)
	}
}

func TestPromiseAwaitUsesPumpContract(t *testing.T) {
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

	calls := 0
	result, err := resolver.Promise().Await(context.Background(), func(context.Context) error {
		calls++
		if calls == 3 {
			return resolver.Resolve(value)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("await promise: %v", err)
	}
	defer result.Release()

	if calls != 3 {
		t.Fatalf("unexpected pump call count: got %d want 3", calls)
	}
	if got := mustString(t, result); got != "done" {
		t.Fatalf("unexpected promise result: %q", got)
	}
}

func TestPromiseAwaitHonorsContextTimeout(t *testing.T) {
	iso := gv8.NewIsolate()
	defer iso.Dispose()

	ctx := gv8.NewContext(iso)
	defer ctx.Close()

	resolver, err := gv8.NewPromiseResolver(ctx)
	if err != nil {
		t.Fatalf("new promise resolver: %v", err)
	}
	defer resolver.Release()

	waitCtx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err = resolver.Promise().Await(waitCtx, nil)
	if err == nil {
		t.Fatalf("expected timeout")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
	if elapsed := time.Since(start); elapsed < 5*time.Millisecond {
		t.Fatalf("await returned too quickly: %s", elapsed)
	}
}

func TestPromiseAwaitRejectedReturnsJSError(t *testing.T) {
	iso := gv8.NewIsolate()
	defer iso.Dispose()

	ctx := gv8.NewContext(iso)
	defer ctx.Close()

	resolver, err := gv8.NewPromiseResolver(ctx)
	if err != nil {
		t.Fatalf("new promise resolver: %v", err)
	}
	defer resolver.Release()

	reason, err := gv8.NewStringValue(ctx, "boom")
	if err != nil {
		t.Fatalf("new string: %v", err)
	}
	defer reason.Release()

	if err := resolver.Reject(reason); err != nil {
		t.Fatalf("reject promise: %v", err)
	}

	_, err = resolver.Promise().Await(context.Background(), nil)
	if err == nil {
		t.Fatalf("expected promise rejection")
	}

	var jsErr *gv8.JSError
	if !errors.As(err, &jsErr) {
		t.Fatalf("expected JSError, got %T", err)
	}
	if jsErr.Message != "promise rejected: boom" {
		t.Fatalf("unexpected rejection message: %q", jsErr.Message)
	}
}

func TestPromiseAwaitExitsOnExecutionTermination(t *testing.T) {
	iso := gv8.NewIsolate()
	defer iso.Dispose()
	defer iso.CancelTerminateExecution()

	ctx := gv8.NewContext(iso)
	defer ctx.Close()

	resolver, err := gv8.NewPromiseResolver(ctx)
	if err != nil {
		t.Fatalf("new promise resolver: %v", err)
	}
	defer resolver.Release()

	termCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	stop := iso.TerminateOnContextDone(termCtx)
	defer stop()

	start := time.Now()
	_, err = resolver.Promise().Await(context.Background(), nil)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("expected error after execution termination")
	}
	var jsErr *gv8.JSError
	if !errors.As(err, &jsErr) {
		t.Fatalf("expected JSError, got %T: %v", err, err)
	}
	if !strings.Contains(jsErr.Message, "terminated") {
		t.Fatalf("unexpected termination message: %q", jsErr.Message)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("await did not exit promptly after termination: %s", elapsed)
	}
}

func TestPromiseAwaitPumpDrivesAsyncGoWork(t *testing.T) {
	iso := gv8.NewIsolate()
	defer iso.Dispose()

	ctx := gv8.NewContext(iso)
	defer ctx.Close()

	resolver, err := gv8.NewPromiseResolver(ctx)
	if err != nil {
		t.Fatalf("new promise resolver: %v", err)
	}
	defer resolver.Release()

	resultCh := make(chan string, 1)
	go func() {
		time.Sleep(10 * time.Millisecond)
		resultCh <- "async-done"
	}()

	result, err := resolver.Promise().Await(context.Background(), func(pumpCtx context.Context) error {
		select {
		case s := <-resultCh:
			v, err := gv8.NewStringValue(ctx, s)
			if err != nil {
				return err
			}
			defer v.Release()
			return resolver.Resolve(v)
		default:
			return nil
		}
	})
	if err != nil {
		t.Fatalf("await: %v", err)
	}
	defer result.Release()

	if got := mustString(t, result); got != "async-done" {
		t.Fatalf("unexpected result: %q", got)
	}
}

func TestPromiseAwaitStressPumpSettlement(t *testing.T) {
	iso := gv8.NewIsolate()
	defer iso.Dispose()

	for i := range 100 {
		ctx := gv8.NewContext(iso)
		resolver, err := gv8.NewPromiseResolver(ctx)
		if err != nil {
			t.Fatalf("iter %d: new promise resolver: %v", i, err)
		}
		v, err := gv8.NewIntegerValue(ctx, int64(i))
		if err != nil {
			t.Fatalf("iter %d: new integer: %v", i, err)
		}

		calls := 0
		result, err := resolver.Promise().Await(context.Background(), func(context.Context) error {
			calls++
			if calls == 2 {
				return resolver.Resolve(v)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("iter %d: await: %v", i, err)
		}
		if got := result.Integer(); got != int64(i) {
			t.Fatalf("iter %d: unexpected result: got %d want %d", i, got, i)
		}
		result.Release()
		v.Release()
		resolver.Release()
		ctx.Close()
	}
}

func TestPromiseAwaitTerminationAndReuseStress(t *testing.T) {
	iso := gv8.NewIsolate()
	defer iso.Dispose()

	for i := range 20 {
		ctx := gv8.NewContext(iso)
		resolver, err := gv8.NewPromiseResolver(ctx)
		if err != nil {
			t.Fatalf("iter %d: new promise resolver: %v", i, err)
		}

		termCtx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
		stop := iso.TerminateOnContextDone(termCtx)

		_, err = resolver.Promise().Await(context.Background(), nil)
		stop()
		cancel()

		if err == nil {
			t.Fatalf("iter %d: expected termination error", i)
		}
		var jsErr *gv8.JSError
		if !errors.As(err, &jsErr) {
			t.Fatalf("iter %d: expected JSError, got %T: %v", i, err, err)
		}
		if !strings.Contains(jsErr.Message, "terminated") {
			t.Fatalf("iter %d: unexpected error message: %q", i, jsErr.Message)
		}

		iso.CancelTerminateExecution()
		resolver.Release()
		ctx.Close()
	}
}

func TestPromiseAwaitFailsWhenContextClosesDuringPendingWork(t *testing.T) {
	iso := gv8.NewIsolate()
	defer iso.Dispose()

	ctx := gv8.NewContext(iso)

	resolver, err := gv8.NewPromiseResolver(ctx)
	if err != nil {
		t.Fatalf("new promise resolver: %v", err)
	}

	promise := resolver.Promise()
	defer resolver.Release()
	defer promise.Release()

	waitDone := make(chan error, 1)
	go func() {
		_, err := promise.Await(context.Background(), func(context.Context) error {
			time.Sleep(time.Millisecond)
			return nil
		})
		waitDone <- err
	}()

	time.Sleep(5 * time.Millisecond)
	ctx.Close()

	select {
	case err := <-waitDone:
		if err == nil {
			t.Fatalf("expected await error after context close")
		}
		var jsErr *gv8.JSError
		if !errors.As(err, &jsErr) {
			t.Fatalf("expected JSError, got %T", err)
		}
		if jsErr.Message != "gv8: value is no longer valid" {
			t.Fatalf("unexpected await error: %q", jsErr.Message)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for await to fail after context close")
	}
}
