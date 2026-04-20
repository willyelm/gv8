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
Native archives are built locally or downloaded as release assets.

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
  internal/v8/      bundled V8 headers, archives, VERSION pin
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
- `make release`

`internal/v8/VERSION` is the single source of truth for the bundled V8 version.
To change V8, update that file and then run:

- `make fetch`

Fetched source lives in `.v8/src/v8`.

Each target build uses its own isolated workspace under `.v8/workspaces/`,
for example:

- `.v8/workspaces/darwin_arm64`
- `.v8/workspaces/linux_x86_64`
- `.v8/workspaces/linux_arm64`

Each platform ends up with one installed native archive:

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

To use a prebuilt release asset instead of building locally:

- `make install-v8`

The installer downloads the matching platform tarball from the GitHub Release
tag `v8-<VERSION>` and extracts the platform runtime into
`internal/v8/<target>/`.

Release asset names follow this format:

- `gv8-v8-<VERSION>-darwin_arm64.tar.gz`
- `gv8-v8-<VERSION>-linux_x86_64.tar.gz`
- `gv8-v8-<VERSION>-linux_arm64.tar.gz`

`make release` packages every already-built platform archive into `dist/` so
you can upload all three assets to a single GitHub Release tagged `v8-<VERSION>`.

The release flow is:

1. build each supported platform
2. run `make release`
3. create or update the GitHub Release tagged `v8-<VERSION>`
4. upload the three files from `dist/`

Current default builds keep optional features off:

- `Intl.*` disabled
- `Temporal` disabled

That keeps the shipped archives small enough for normal Git storage and keeps
the build close to V8's normal public build flow.
