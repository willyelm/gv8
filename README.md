# gv8

[![ci](https://img.shields.io/github/actions/workflow/status/willyelm/gv8/ci.yml?branch=main&style=flat-square&label=ci)](https://github.com/willyelm/gv8/actions/workflows/ci.yml)
[![V8 14.8.178.21](https://img.shields.io/badge/V8-14.8.178.21%20%40%20e38030f-blue?style=flat-square)](https://chromium.googlesource.com/v8/v8.git/+/e38030f4228c8d1405fe105fc5feaa5173559e25)

`gv8` embeds the V8 JavaScript engine in Go.

It is a small binding layer for hosts that want to execute JavaScript and
WebAssembly through V8 directly. It does not try to be a JavaScript runtime,
server framework, package loader, or browser-like host.

The value of `gv8` is direct access to V8 concepts from Go with explicit
ownership and low binding overhead.

## What You Get

- isolates and contexts
- scripts and ES modules
- static and dynamic module resolution
- promises and microtask checkpoints
- host functions
- `ArrayBuffer` and typed array byte movement
- V8 snapshots
- persistent handles
- WebAssembly through V8's standard `WebAssembly` APIs
- execution termination, heap limits, memory pressure, and heap statistics
- lightweight hooks for exceptions, promise rejections, and module resolution failures

## Scope

`gv8` stays close to V8:

- `Isolate` owns V8 state.
- `Context` is an execution environment.
- `Module` and `UnboundScript` represent compiled JavaScript.
- `Value`, `Object`, `Function`, and `Promise` wrap V8 handles.
- `Close`, `Dispose`, and `Release` are part of normal ownership.

Host policy stays outside `gv8`. Scheduling, filesystem access, networking,
permissions, package loading, and application lifecycle belong to the consumer.

## Quick Start

```go
iso := gv8.NewIsolate()
defer iso.Dispose()

ctx := gv8.NewContext(iso)
defer ctx.Close()

value, err := ctx.RunScript("40 + 2", "main.js")
if err != nil {
	panic(err)
}
defer value.Release()

fmt.Println(value.Integer())
```

## ESM Loading

`gv8` compiles and evaluates ES modules, but the host owns resolution. A
resolver can load modules from memory, disk, a database, or any application
source.

```go
type modules map[string]string

func (m modules) ResolveModule(ctx *gv8.Context, spec string, _ *gv8.Module) (*gv8.Module, error) {
	source, ok := m[spec]
	if !ok {
		return nil, fmt.Errorf("module not found: %s", spec)
	}
	return ctx.Isolate().CompileModule(source, gv8.ScriptOrigin{
		ResourceName: spec,
		IsModule:     true,
	})
}

resolver := modules{
	"./math.js": `export function add(a, b) { return a + b }`,
}

mod, err := iso.CompileModule(`
	import { add } from "./math.js";
	export const answer = add(20, 22);
`, gv8.ScriptOrigin{ResourceName: "app.js", IsModule: true})
if err != nil {
	panic(err)
}
defer mod.Release()

if err := mod.Instantiate(ctx, resolver); err != nil {
	panic(err)
}
result, err := mod.Evaluate(ctx)
if err != nil {
	panic(err)
}
defer result.Release()
```

## Performance Shape

Most execution performance comes from V8. `gv8` keeps the Go side explicit so
hosts can avoid unnecessary work: reuse isolates where appropriate, compile once
when running the same source repeatedly, move bytes without JSON, and preload
common JavaScript with snapshots.

```go
script, err := iso.CompileUnboundScript(`
	let total = 0;
	for (const byte of input) total += byte;
	total;
`, gv8.ScriptOrigin{
	ResourceName: "checksum.js",
})
if err != nil {
	panic(err)
}
defer script.Release()

global := ctx.Global()
defer global.Release()

for _, payload := range payloads {
	input, err := gv8.NewUint8ArrayCopy(ctx, payload)
	if err != nil {
		panic(err)
	}

	if err := global.Set("input", input); err != nil {
		input.Release()
		panic(err)
	}
	input.Release()

	result, err := script.Run(ctx)
	if err != nil {
		panic(err)
	}
	result.Release()
}
```

For faster isolate startup, build a V8 snapshot from trusted boot code:

```go
builder := gv8.NewSnapshotBuilder()
defer builder.Release()

if err := builder.RunScript(`globalThis.config = Object.freeze({ version: 1 })`, gv8.ScriptOrigin{
	ResourceName: "boot.js",
}); err != nil {
	panic(err)
}

snapshot, err := builder.Build()
if err != nil {
	panic(err)
}

iso := gv8.NewIsolateWithOptions(gv8.IsolateOptions{Snapshot: snapshot})
defer iso.Dispose()
```

## WebAssembly

JavaScript running in V8 can instantiate WebAssembly directly. The host loads
the `.wasm` bytes, passes them into V8 as a typed array, and provides any JS
glue or imports the module expects.

```go
wasmBytes, err := os.ReadFile("plugin.wasm")
if err != nil {
	panic(err)
}

bytes, err := gv8.NewUint8ArrayCopy(ctx, wasmBytes)
if err != nil {
	panic(err)
}
defer bytes.Release()

global := ctx.Global()
defer global.Release()

if err := global.Set("wasmBytes", bytes); err != nil {
	panic(err)
}

value, err := ctx.RunScript(`
	const imports = {
		env: {
			scale: value => value * 2,
		},
	};

	const module = new WebAssembly.Module(wasmBytes);
	const instance = new WebAssembly.Instance(module, imports);

	instance.exports.score(21);
`, "wasm.js")
if err != nil {
	panic(err)
}
defer value.Release()
```

Larger WASM packages often ship with JavaScript initialization code and required
imports. `gv8` exposes V8's WASM execution path; the host remains responsible
for loading files, choosing imports, and running any package-specific glue.

## Operational Rules

- One isolate may have only one active caller at a time.
- Reentrant calls on the same thread are allowed.
- Different isolates may run concurrently.
- Handles belong to their original isolate and context lineage.
- Persistent handles keep values alive but do not move them between contexts.
- Snapshot data must come from `gv8` or another trusted V8 build with the same
  V8 version.
- JavaScript exceptions are returned as `JSError`.

## Supported Platforms

Bundled V8 artifacts are committed for:

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
