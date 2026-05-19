# TODO

Production-readiness work for `gv8`, keeping the core intentionally small and focused on a server-side JS host runtime.

1. Fix lifetime ownership
   - Add explicit `Release`/`Close` for `Module` and `UnboundScript`.
   - Make `Context.Close()` invalidate Go wrappers safely.
   - Add teardown tests for use-after-close and double-release.

2. Eliminate global registration leaks
   - Remove or unregister `hostFnRefs` entries when no longer needed.
   - Ensure context registry and resolver state are fully cleaned on close.
   - Add leak tests for repeated create/destroy cycles.

3. Define a strict threading model
   - Decide whether isolates are thread-affine or externally synchronized.
   - Enforce the contract in Go with runtime checks.
   - Document which operations are safe concurrently.

4. Make error behavior non-panicking
   - Replace panic-based conversion paths with error-returning APIs or safe variants.
   - Normalize JS exception reporting through `JSError`.

5. Harden promise and microtask execution
   - Replace busy-spin `Await()` with a host-driven wait strategy.
   - Expose a simple event-loop or pump contract.
   - Support timeout and cancellation without burning CPU.

6. Isolate module loading state
   - Stop storing one mutable resolver on `Context`.
   - Bind resolver state to a module graph or evaluation session.
   - Add tests for interleaved and concurrent module loading.

7. Add execution control primitives
   - Support execution termination and timeouts per isolate.
   - Support memory limits and basic heap pressure reporting.
   - Expose the minimum controls needed to kill runaway code.

8. Add observability
   - Expose V8 version, heap stats, and basic counters.
   - Surface unhandled promise rejections.
   - Add optional hooks for exceptions and resolver failures.

9. Tighten API shape around the simple core
   - Keep the surface focused on `Isolate`, `Context`, `Script`, `Module`, `Value`, `Function`, and `Promise`.
   - Prefer explicit ownership over convenience APIs that hide lifecycle rules.

10. Support server-runtime essentials only
   - Stable host function callbacks.
   - ESM loading with static and dynamic import.
   - JSON and byte access.
   - Promise resolution and microtask pumping.
   - Optional snapshots only if startup latency justifies the complexity.

11. Improve test depth, not just breadth
   - Add stress tests for repeated create/run/dispose cycles.
   - Add concurrency tests around isolate and context access.
   - Add failure-path tests for exceptions, promise rejection, resolver failure, and teardown during pending work.

12. Add a minimal CI matrix
   - Test supported targets on every change.
   - Run leak and stress tests separately.
   - Make `make test` self-contained and independent of host cache paths.

13. Clarify operational guarantees in the README
   - State supported platforms.
   - State the threading model.
   - State ownership rules for each handle type.
   - State what production support means and what is intentionally out of scope.

