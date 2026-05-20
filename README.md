# gv8

[![ci](https://github.com/willyelm/gv8/actions/workflows/ci.yml/badge.svg?style=flat)](https://github.com/willyelm/gv8/actions/workflows/ci.yml)
[![V8 14.8.178.21](https://img.shields.io/badge/V8-14.8.178.21%20%40%20e38030f-blue?style=flat)](https://chromium.googlesource.com/v8/v8.git/+/e38030f4228c8d1405fe105fc5feaa5173559e25)

`gv8` embeds the V8 JavaScript engine in Go.

It exposes a small Go-facing API for running modern JavaScript inside V8 while
staying close to V8's own execution model.

The goal is a compact, predictable binding for Go hosts that want to execute
JavaScript through V8 directly. `gv8` keeps embedding small, explicit, and
efficient.

## What It Supports

- isolates and contexts
- scripts and ES modules
- static and dynamic module loading
- promises and microtask pumping
- host function bindings
- JSON helpers
- `ArrayBuffer` and typed array byte movement
- V8 snapshot creation and isolate creation from snapshots
- persistent handles for long-lived values and functions
- JavaScript-hosted WebAssembly execution through V8's standard `WebAssembly` APIs
- execution termination, heap limits, and memory pressure signals
- lightweight observability for exceptions, promise rejection, and module resolution failures

## Good Fits

- custom execution environments
- plugin systems
- Go applications that need direct V8 execution

## Design Goals

- stay close to V8 concepts instead of inventing a large Go-side framework
- keep ownership explicit
- keep isolate behavior predictable under load
- keep the API surface compact
- keep binding overhead low
- expose V8 concepts without absorbing host semantics

## Core API

The intended long-lived surface is:

- `Isolate`
- `Context`
- `UnboundScript`
- `Module`
- `Value`
- `Function`
- `Promise`

Prefer the explicit APIs built around those types. Some convenience helpers
remain for ergonomics, but they are secondary to the ownership-first path.

## Operational Model

- One isolate may have only one active caller at a time.
- Reentrant calls on that same thread are allowed.
- Different isolates may run concurrently.
- Every handle belongs to one isolate and one context lineage.
- Persistent handles keep V8 values alive, but they still belong to their
  original context lineage.
- Snapshot data must come from `gv8`'s snapshot builder or another trusted V8
  source built for the same V8 version.
- `Close`, `Dispose`, and `Release` are part of normal ownership.
- APIs returning `error` surface JavaScript exceptions as `JSError`.

In practice, treat an `Isolate` and everything created from it as a
single-threaded unit and serialize access in the host.

## Runtime Scope

`gv8` is intentionally narrow. It focuses on direct V8 execution primitives:

- ES module execution
- resolver-driven loading
- host callbacks
- promise integration
- execution control
- snapshot creation and loading
- JavaScript-driven WebAssembly execution

It does not try to provide full V8 surface parity, a browser-style host, or
application runtime behavior above V8.

## Example

```go
iso := gv8.NewIsolate()
defer iso.Dispose()

ctx := gv8.NewContext(iso)
defer ctx.Close()

value, err := ctx.RunScript("40 + 2", "example.js")
if err != nil {
	panic(err)
}
defer value.Release()

println(value.Integer())
```

## API Overview

### Isolates And Contexts

An `Isolate` owns V8 state. A `Context` is an execution environment inside that
isolate.

```go
iso := gv8.NewIsolate()
defer iso.Dispose()

ctx := gv8.NewContext(iso)
defer ctx.Close()
```

### Scripts

Use `RunScript` for one-off execution, or compile an `UnboundScript` if you
want to run the same source more than once.

```go
value, err := ctx.RunScript("6 * 7", "main.js")
if err != nil {
	panic(err)
}
defer value.Release()
```

```go
script, err := iso.CompileUnboundScript("40 + 2", gv8.ScriptOrigin{
	ResourceName: "compiled.js",
})
if err != nil {
	panic(err)
}
defer script.Release()
```

### ES Modules

`gv8` supports compiling, instantiating, evaluating, and reading exports from
ES modules.

```go
mod, err := iso.CompileModule(`export const answer = 42;`, gv8.ScriptOrigin{
	ResourceName: "mod.js",
	IsModule:     true,
})
if err != nil {
	panic(err)
}
defer mod.Release()

if err := mod.Instantiate(ctx, nil); err != nil {
	panic(err)
}
if _, err := mod.Evaluate(ctx); err != nil {
	panic(err)
}
```

### Module Resolution

Imports are resolved through a host-provided resolver. Static imports resolve
by referrer module identity, and dynamic imports resolve by importer resource
name.

```go
type resolver struct {
	modules map[string]string
	cache   map[string]*gv8.Module
}

func (r *resolver) ResolveModule(ctx *gv8.Context, spec string, _ *gv8.Module) (*gv8.Module, error) {
	if r.cache == nil {
		r.cache = map[string]*gv8.Module{}
	}
	if mod, ok := r.cache[spec]; ok {
		return mod, nil
	}

	mod, err := ctx.Isolate().CompileModule(r.modules[spec], gv8.ScriptOrigin{
		ResourceName: spec,
		IsModule:     true,
	})
	if err != nil {
		return nil, err
	}
	r.cache[spec] = mod
	return mod, nil
}
```

```go
resolver := &resolver{
	modules: map[string]string{
		"./dep.js": `export const answer = 41;`,
	},
}

mod, err := iso.CompileModule(`
	import { answer } from "./dep.js";
	export const value = answer + 1;
`, gv8.ScriptOrigin{ResourceName: "main.js", IsModule: true})
if err != nil {
	panic(err)
}
defer mod.Release()

if err := mod.Instantiate(ctx, resolver); err != nil {
	panic(err)
}
```

### Dynamic Import

If your resolver also implements `DynamicImportResolver`, it can resolve
`import(...)` at runtime by returning a promise for the target module namespace.

```go
func (r *resolver) ResolveDynamicImport(ctx *gv8.Context, spec string, _ string) (*gv8.Promise, error) {
	resolver, err := gv8.NewPromiseResolver(ctx)
	if err != nil {
		return nil, err
	}

	mod, err := r.ResolveModule(ctx, spec, nil)
	if err != nil {
		return nil, err
	}
	if err := mod.Instantiate(ctx, r); err == nil && mod.Status() == gv8.ModuleStatusInstantiated {
		_, err = mod.Evaluate(ctx)
	}
	if err != nil {
		return nil, err
	}

	if err := resolver.Resolve(mod.Namespace(ctx).Value); err != nil {
		return nil, err
	}
	return resolver.Promise(), nil
}
```

### Host Functions

Use host functions to expose Go logic to JavaScript.

```go
add, err := gv8.NewFunction(ctx, func(info *gv8.FunctionCallbackInfo) (*gv8.Value, error) {
	args := info.Args()
	return gv8.NewIntegerValue(info.Context(), args[0].Integer()+args[1].Integer())
})
if err != nil {
	panic(err)
}
defer add.Release()

global := ctx.Global()
defer global.Release()

if err := global.Set("add", add); err != nil {
	panic(err)
}
```

### Promises

`Promise.Await()` waits for JavaScript promises from Go. It performs microtask
checkpoints and can use a host-provided pump callback.

```go
resolver, err := gv8.NewPromiseResolver(ctx)
if err != nil {
	panic(err)
}
defer resolver.Release()

value, _ := gv8.NewStringValue(ctx, "done")
defer value.Release()

go func() {
	_ = resolver.Resolve(value)
}()

result, err := resolver.Promise().Await(context.Background(), nil)
if err != nil {
	panic(err)
}
defer result.Release()
```

### Execution Control

You can terminate runaway execution, inspect heap limits, and attach isolate
termination to a host context.

```go
iso := gv8.NewIsolateWithOptions(gv8.IsolateOptions{
	InitialHeapSizeBytes: 8 << 20,
	MaxHeapSizeBytes:     32 << 20,
})
defer iso.Dispose()

stats := iso.HeapStatistics()
_ = stats.HeapSizeLimit
```

### Observability

The isolate can report unhandled promise rejections, surfaced JavaScript
exceptions, and module resolution failures.

```go
iso.SetExceptionHandler(func(err *gv8.JSError) {
	log.Printf("exception: %s", err.Message)
})

iso.SetModuleResolutionFailureHandler(func(specifier, referrer string, err error) {
	log.Printf("module resolution failed: %s from %s: %v", specifier, referrer, err)
})
```

### Bytes And Typed Arrays

`gv8` can move bytes through V8 `ArrayBuffer` and typed array values.

```go
value, err := gv8.NewUint8ArrayCopy(ctx, []byte("HI"))
if err != nil {
	panic(err)
}
defer value.Release()

data, err := value.Bytes()
if err != nil {
	panic(err)
}
```

### Snapshots

Snapshots expose V8's startup snapshot mechanism directly. `gv8` does not
decide what to preload; it only builds and consumes V8 snapshot data.

```go
builder := gv8.NewSnapshotBuilder()
defer builder.Release()

if err := builder.RunScript(`globalThis.answer = 42`, gv8.ScriptOrigin{
	ResourceName: "snapshot.js",
}); err != nil {
	panic(err)
}

snapshot, err := builder.Build()
if err != nil {
	panic(err)
}

iso := gv8.NewIsolateWithOptions(gv8.IsolateOptions{
	Snapshot: snapshot,
})
defer iso.Dispose()
```

### Persistent Handles

Use persistent handles for V8 values that must outlive a temporary Go wrapper.
They do not change ownership rules: the value still belongs to its original
context and isolate.

```go
fn, err := gv8.NewFunction(ctx, func(info *gv8.FunctionCallbackInfo) (*gv8.Value, error) {
	return gv8.NewStringValue(info.Context(), "ok")
})
if err != nil {
	panic(err)
}
defer fn.Release()

handle, err := gv8.NewGlobalFunction(fn)
if err != nil {
	panic(err)
}
defer handle.Release()
```

### WebAssembly

JavaScript running inside `gv8` can use V8's standard `WebAssembly` APIs.

```go
value, err := ctx.RunScript(`
	const wasmBytes = new Uint8Array([
		0x00, 0x61, 0x73, 0x6d,
		0x01, 0x00, 0x00, 0x00,
		0x01, 0x05, 0x01, 0x60, 0x00, 0x01, 0x7f,
		0x03, 0x02, 0x01, 0x00,
		0x07, 0x0a, 0x01, 0x06, 0x61, 0x6e, 0x73, 0x77, 0x65, 0x72, 0x00, 0x00,
		0x0a, 0x06, 0x01, 0x04, 0x00, 0x41, 0x2a, 0x0b,
	]);

	const mod = new WebAssembly.Module(wasmBytes);
	const instance = new WebAssembly.Instance(mod);
	instance.exports.answer();
`, "wasm.js")
if err != nil {
	panic(err)
}
defer value.Release()
```

## Supported Platforms

Bundled runtimes are committed for:

- `darwin/arm64`
- `linux/amd64`
- `linux/arm64`

Runtime artifacts:

- `internal/v8/darwin_arm64/libgv8.dylib`
- `internal/v8/linux_x86_64/libgv8.so`
- `internal/v8/linux_arm64/libgv8.so`

## Maintenance

`internal/v8/SOURCE_COMMIT` is the source of truth for the bundled V8 source.

Maintainer flow:

- `make fetch`
- `make build`
- `make build-linux-x64`
- `make build-linux-arm64`

Useful test targets:

- `make test`
- `make test-leak`
- `make test-stress`
- `make ci-test`
