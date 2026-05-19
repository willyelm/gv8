package gv8_test

import (
	"context"
	"testing"

	"github.com/willyelm/gv8"
)

func TestModuleNamespace(t *testing.T) {
	iso := gv8.NewIsolate()
	defer iso.Dispose()

	ctx := gv8.NewContext(iso)
	defer ctx.Close()

	mod, err := iso.CompileModule("export const answer = 42;", gv8.ScriptOrigin{
		ResourceName: "mod.js",
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

func TestModuleTopLevelAwaitNamespace(t *testing.T) {
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
	})
	if err != nil {
		t.Fatalf("compile module: %v", err)
	}
	defer mod.Release()

	if err := mod.Instantiate(ctx, resolver); err != nil {
		t.Fatalf("instantiate module: %v", err)
	}
	evalResult, err := mod.Evaluate(ctx)
	if err != nil {
		t.Fatalf("evaluate module: %v", err)
	}
	if evalResult != nil && evalResult.IsPromise() {
		if _, err := evalResult.Promise().Await(context.Background(), nil); err != nil {
			t.Fatalf("await module: %v", err)
		}
		evalResult.Release()
	}
	namespace := mod.Namespace(ctx)

	exported, err := namespace.Get("value")
	if err != nil {
		t.Fatalf("get export: %v", err)
	}
	defer exported.Release()

	if got := exported.Integer(); got != 42 {
		t.Fatalf("unexpected export: got %d want 42", got)
	}
}

func TestModuleResolverIsolationByReferrer(t *testing.T) {
	iso := gv8.NewIsolate()
	defer iso.Dispose()

	ctx := gv8.NewContext(iso)
	defer ctx.Close()

	leftResolver := &isolatedResolver{value: 41}
	rightResolver := &isolatedResolver{value: 99}

	left, err := iso.CompileModule(`
		import { answer } from "./dep.js";
		export const value = answer + 1;
	`, gv8.ScriptOrigin{ResourceName: "left.js"})
	if err != nil {
		t.Fatalf("compile left module: %v", err)
	}
	defer left.Release()

	right, err := iso.CompileModule(`
		import { answer } from "./dep.js";
		export const value = answer + 1;
	`, gv8.ScriptOrigin{ResourceName: "right.js"})
	if err != nil {
		t.Fatalf("compile right module: %v", err)
	}
	defer right.Release()

	if err := left.Instantiate(ctx, leftResolver); err != nil {
		t.Fatalf("instantiate left: %v", err)
	}
	if err := right.Instantiate(ctx, rightResolver); err != nil {
		t.Fatalf("instantiate right: %v", err)
	}

	if _, err := left.Evaluate(ctx); err != nil {
		t.Fatalf("evaluate left: %v", err)
	}
	if _, err := right.Evaluate(ctx); err != nil {
		t.Fatalf("evaluate right: %v", err)
	}
	leftValue := left.Namespace(ctx)
	rightValue := right.Namespace(ctx)

	gotLeft, err := leftValue.Get("value")
	if err != nil {
		t.Fatalf("get left value: %v", err)
	}
	defer gotLeft.Release()
	gotRight, err := rightValue.Get("value")
	if err != nil {
		t.Fatalf("get right value: %v", err)
	}
	defer gotRight.Release()

	if got := gotLeft.Integer(); got != 42 {
		t.Fatalf("unexpected left export: got %d want 42", got)
	}
	if got := gotRight.Integer(); got != 100 {
		t.Fatalf("unexpected right export: got %d want 100", got)
	}
}

func TestDynamicImportResolverIsolationByResourceName(t *testing.T) {
	iso := gv8.NewIsolate()
	defer iso.Dispose()

	ctx := gv8.NewContext(iso)
	defer ctx.Close()

	leftResolver := &isolatedResolver{value: 41}
	rightResolver := &isolatedResolver{value: 99}

	left, err := iso.CompileModule(`
		export const value = await import("./dep.js").then((mod) => mod.answer + 1);
	`, gv8.ScriptOrigin{ResourceName: "left-dynamic.js"})
	if err != nil {
		t.Fatalf("compile left module: %v", err)
	}
	defer left.Release()

	right, err := iso.CompileModule(`
		export const value = await import("./dep.js").then((mod) => mod.answer + 1);
	`, gv8.ScriptOrigin{ResourceName: "right-dynamic.js"})
	if err != nil {
		t.Fatalf("compile right module: %v", err)
	}
	defer right.Release()

	if err := left.Instantiate(ctx, leftResolver); err != nil {
		t.Fatalf("instantiate left: %v", err)
	}
	leftEval, err := left.Evaluate(ctx)
	if err != nil {
		t.Fatalf("evaluate left: %v", err)
	}
	if leftEval != nil && leftEval.IsPromise() {
		if _, err := leftEval.Promise().Await(context.Background(), nil); err != nil {
			t.Fatalf("await left: %v", err)
		}
		leftEval.Release()
	}
	leftNS := left.Namespace(ctx)

	if err := right.Instantiate(ctx, rightResolver); err != nil {
		t.Fatalf("instantiate right: %v", err)
	}
	rightEval, err := right.Evaluate(ctx)
	if err != nil {
		t.Fatalf("evaluate right: %v", err)
	}
	if rightEval != nil && rightEval.IsPromise() {
		if _, err := rightEval.Promise().Await(context.Background(), nil); err != nil {
			t.Fatalf("await right: %v", err)
		}
		rightEval.Release()
	}
	rightNS := right.Namespace(ctx)

	gotLeft, err := leftNS.Get("value")
	if err != nil {
		t.Fatalf("get left value: %v", err)
	}
	defer gotLeft.Release()
	gotRight, err := rightNS.Get("value")
	if err != nil {
		t.Fatalf("get right value: %v", err)
	}
	defer gotRight.Release()

	if got := gotLeft.Integer(); got != 42 {
		t.Fatalf("unexpected left export: got %d want 42", got)
	}
	if got := gotRight.Integer(); got != 100 {
		t.Fatalf("unexpected right export: got %d want 100", got)
	}
}

func TestModuleRelease(t *testing.T) {
	iso := gv8.NewIsolate()
	defer iso.Dispose()

	mod, err := iso.CompileModule("export const answer = 42;", gv8.ScriptOrigin{
		ResourceName: "release-module.js",
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
