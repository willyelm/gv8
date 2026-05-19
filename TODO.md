# TODO

Production-readiness work for `gv8`, keeping the core intentionally small and focused on a server-side JS host runtime.

[x] 1. Fix lifetime ownership
- [x] Add explicit `Release`/`Close` for `Module` and `UnboundScript`.
- [x] Make `Context.Close()` invalidate Go wrappers safely.
- [x] Route fresh native values through tracked constructors.
- [x] Add teardown tests for use-after-close and double-release.
- [x] Add stress tests for repeated create/run/close cycles.

[x] 2. Eliminate global registration leaks
- [x] Remove or unregister `hostFnRefs` entries when no longer needed.
- [x] Ensure context registry and resolver state are fully cleaned on close.
- [x] Add leak tests for repeated create/destroy cycles.

[x] 3. Define a strict threading model
- [x] Decide whether isolates are thread-affine or externally synchronized.
- [x] Enforce the contract in Go with runtime checks.
- [x] Document which operations are safe concurrently.

[x] 4. Make error behavior non-panicking
- [x] Replace panic-based conversion paths with error-returning APIs or safe variants.
- [x] Normalize JS exception reporting through `JSError`.

[x] 5. Harden promise and microtask execution
- [x] Replace busy-spin `Await()` with a host-driven wait strategy.
- [x] Expose a simple event-loop or pump contract.
- [x] Support timeout and cancellation without burning CPU.

[x] 6. Isolate module loading state
- [x] Stop storing one mutable resolver on `Context`.
- [x] Bind resolver state to a module graph or evaluation session.
- [x] Add tests for interleaved and concurrent module loading.

[x] 7. Add execution control primitives
- [x] Support execution termination and timeouts per isolate.
- [x] Support memory limits and basic heap pressure reporting.
- [x] Expose the minimum controls needed to kill runaway code.

[x] 8. Add observability
- [x] Expose V8 version, heap stats, and basic counters.
- [x] Surface unhandled promise rejections.
- [x] Add optional hooks for exceptions and resolver failures.

[ ] 9. Tighten API shape around the simple core
- [ ] Keep the surface focused on `Isolate`, `Context`, `Script`, `Module`, `Value`, `Function`, and `Promise`.
- [ ] Prefer explicit ownership over convenience APIs that hide lifecycle rules.

[ ] 10. Support server-runtime essentials only
- [ ] Stable host function callbacks.
- [ ] ESM loading with static and dynamic import.
- [ ] JSON and byte access.
- [ ] Promise resolution and microtask pumping.
- [ ] Optional snapshots only if startup latency justifies the complexity.

[ ] 11. Improve test depth, not just breadth
- [x] Add stress tests for repeated create/run/dispose cycles.
- [ ] Add concurrency tests around isolate and context access.
- [ ] Add failure-path tests for exceptions, promise rejection, resolver failure, and teardown during pending work.

[ ] 12. Add a minimal CI matrix
- [ ] Test supported targets on every change.
- [ ] Run leak and stress tests separately.
- [ ] Make `make test` self-contained and independent of host cache paths.

[ ] 13. Clarify operational guarantees in the README
- [ ] State supported platforms.
- [ ] State the threading model.
- [ ] State ownership rules for each handle type.
- [ ] State what production support means and what is intentionally out of scope.
