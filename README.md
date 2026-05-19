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

## Error Handling

`gv8` reports JavaScript exceptions as `JSError`.

- APIs that already return `error` use `JSError` for V8 exceptions.
- `Value.StringValue()` returns `(string, error)` and reports conversion
  failures as `JSError`.
- `Value.String()` is a convenience helper that returns `""` on conversion
  failure instead of panicking.
- Promise rejection surfaced by `Promise.Await()` is returned as `JSError`.

## Promise Waiting

`Promise.Await()` is safe for server runtimes by default.

- It performs a microtask checkpoint before each wait iteration.
- It does not busy-spin when no host pump is provided.
- It honors context cancellation and deadlines.
- The `pump` callback is the host event-loop integration point.

If you need more control, `Promise.AwaitWithOptions()` lets you provide:

- a `Pump` callback for host-driven progress
- a `PollInterval` for default wait behavior when no pump is used

## Module Resolution

Module resolvers in `gv8` are bound to module graphs, not stored as one mutable
context-global resolver.

- Static imports resolve by referrer module identity.
- Dynamic imports resolve by importer resource name.
- Multiple independent module graphs can coexist in one context without
  overwriting each other's resolver state.

## Execution Control

`gv8` exposes the minimum isolate controls needed for a server runtime:

- `TerminateExecution()` to stop runaway JavaScript
- `CancelTerminateExecution()` to resume execution capability after handling a
  termination event
- `IsExecutionTerminating()` to inspect termination state
- `TerminateOnContextDone(ctx)` to connect isolate termination to host
  cancellation or deadlines
- `HeapStatistics()` for basic memory reporting
- `LowMemoryNotification()` and `MemoryPressureNotification(...)` for heap
  pressure signaling
- `NewIsolateWithOptions(...)` to configure initial and maximum heap sizes

## Observability

`gv8` exposes lightweight isolate-level observability hooks and counters.

- `SetUnhandledPromiseRejectionHandler(...)`
- `SetExceptionHandler(...)`
- `SetModuleResolutionFailureHandler(...)`
- `ObservabilitySnapshot()`

The snapshot currently tracks:

- unhandled promise rejections
- uncaught exception messages
- module resolution failures

These hooks are intended for host logging, metrics, and diagnostics without
requiring the V8 inspector.

## Threading

`gv8` uses an externally synchronized isolate model.

- One isolate may have only one active caller at a time.
- Reentrant calls on that same thread are allowed so host callbacks can call
  back into `gv8`.
- Concurrent use of the same isolate, context, or values from multiple threads
  is rejected at runtime.
- Different isolates may be used concurrently.

In practice:

- treat an `Isolate` and everything created from it as a single-threaded unit
- serialize access in your host runtime
- expect methods that return `error` to report concurrent access violations
- expect some non-error convenience methods to panic if you violate the contract

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
