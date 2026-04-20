# gv8

`gv8` is a small Go embedding toolkit for V8.

The design goal is to stay close to V8's design model:

- `Isolate` and `Context` ownership
- Script and module compilation
- Module instantiation and evaluation
- Module namespace exports from Go
- Minimal V8 API parity

This version keeps the API intentionally small:

- `Isolate`
- `Context`
- `UnboundScript`
- `Module`
- `Value` / `Object`
- `ModuleResolver`
- `JSError`

Implemented in this bootstrap:

- unbound script compilation and execution
- ESM compilation, instantiation, evaluation, and namespace export access
- resolver-driven module loading
- explicit context ownership and microtask checkpoints

Planned next:

- host functions and host-owned values
- dynamic import hooks
- snapshots

## Status

This repository vendors V8 headers and version metadata under `internal/v8`.
Native runtimes are committed in-module per supported platform.

## Maintainer

Will Medina <williams.medinaa@gmail.com>

## Example

```go
iso := gv8.NewIsolate()
defer iso.Dispose()

ctx := gv8.NewContext(iso)
defer ctx.Close()

script, err := iso.CompileUnboundScript("40 + 2", gv8.ScriptOrigin{
	ResourceName: "example.js",
})
if err != nil {
	panic(err)
}

value, err := script.Run(ctx)
if err != nil {
	panic(err)
}

println(value.Integer())
```

## Layout

```text
gv8/
  *.go              public API
  gv8.h             bridge API shared with cgo
  internal/v8/      bundled V8 headers, runtimes, VERSION pin
  internal/gv8.cc   native bridge built with V8 toolchain
  scripts/          maintainer scripts
  Makefile          thin convenience wrapper
```

## Maintenance

The maintainer flow stays explicit:

- `make fetch`
- `make build`
- `make build-linux-x64`
- `make build-linux-arm64`

`internal/v8/VERSION` is the single source of truth for the bundled V8 version.
To change V8, update that file and then run:

- `make fetch`

Fetched source lives in `.v8/src/v8`.

Each target build uses its own isolated workspace under `.v8/workspaces/`,
for example:

- `.v8/workspaces/darwin_arm64`
- `.v8/workspaces/linux_x86_64`
- `.v8/workspaces/linux_arm64`

Each platform ends up with one native runtime:

- `internal/v8/darwin_arm64/libgv8.dylib`
- `internal/v8/linux_x86_64/libgv8.so`
- `internal/v8/linux_arm64/libgv8.so`

The fetch step is a source-only fast path:

1. clone `v8.git`
2. `git checkout --detach <VERSION>`
3. copy `include/` and version metadata into `internal/v8/`

The build step stays minimal and target-scoped. During `make build`, the
script creates or reuses the target workspace, installs `depot_tools`, runs
`gclient sync`, builds `v8_monolith` and `v8_libplatform`, compiles the small
`gv8` bridge with the same V8 toolchain, and links a single shared `libgv8`
runtime for the target platform. When ICU data is built as an external file,
the build also installs `icudtl.dat` beside the shared library.

Current default builds keep key platform features enabled:

- `Intl.*` enabled
- `Temporal` enabled
- sandbox enabled

The repository ships the built runtimes directly so downstream Go builds do not
need a separate native install step.
