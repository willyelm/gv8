// Package gv8 is a focused V8 embedding layer for Go hosts that need direct,
// predictable JavaScript execution through V8.
//
// The package is intentionally built around a narrow core surface:
//
//   - Isolate
//   - Context
//   - UnboundScript
//   - Module
//   - Value
//   - Function
//   - Promise
//
// Everything created from an Isolate should be treated as belonging to that
// isolate's ownership domain and threading domain.
//
// The design goal is a compact execution core with explicit control over
// ownership, scheduling, and teardown. It favors low-overhead primitives over
// convenience abstractions that hide lifecycle costs, and it is suitable for
// custom execution environments and plugin systems. JavaScript running inside
// V8 also has access to V8's standard WebAssembly APIs.
//
// Prefer the explicit APIs that make ownership and lifecycle visible:
//
//   - Isolate -> NewContext / CompileUnboundScript / CompileModule
//   - Context -> Global / NewObject / RunScript
//   - Value -> Release
//   - Module / UnboundScript -> Release
//
// Convenience helpers remain available, but they are secondary to the explicit
// APIs and should not define host execution policy.
//
// Intentionally out of scope: broad runtime behavior, wide V8 API parity, or
// heavy host abstractions that trade clarity for surface area.
package gv8
