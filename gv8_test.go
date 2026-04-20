package gv8_test

import (
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

	value, err := script.Run(ctx)
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
