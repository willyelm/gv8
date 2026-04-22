# gv8

`gv8` is a small Go binding for embedding V8.

The project stays close to V8's model instead of adding a large Go-specific
runtime layer. The core API is built around:

- `Isolate`
- `Context`
- `UnboundScript`
- `Module`
- `Value` / `Object` / `Function` / `Promise`
- `ModuleResolver`
- `JSError`

Current support includes:

- script compilation and execution
- ESM compilation, instantiation, evaluation, and namespace access
- resolver-driven module loading
- dynamic import callbacks
- host functions
- object/property helpers
- JSON helpers
- promise resolution and awaiting

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

println(value.Integer())
```

## Bundled Runtime

`gv8` vendors V8 headers, version metadata, and prebuilt runtimes in-module.
The supported targets are:

- `darwin/arm64`
- `linux/amd64`
- `linux/arm64`

Each target ships one runtime:

- `internal/v8/darwin_arm64/libgv8.dylib`
- `internal/v8/linux_x86_64/libgv8.so`
- `internal/v8/linux_arm64/libgv8.so`

## Maintenance

`internal/v8/VERSION` is the single source of truth for the bundled V8 version.

Maintainer flow:

- `make fetch`
- `make build`
- `make build-linux-x64`
- `make build-linux-arm64`

`make fetch` updates the pinned V8 source.

The build targets reuse isolated workspaces under `.v8/workspaces/` and produce
the bundled runtimes committed under `internal/v8/`.
