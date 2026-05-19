// Package gv8 is a small Go binding for embedding V8.
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
// Prefer the explicit APIs that make ownership and lifecycle visible:
//
//   - Isolate -> NewContext / CompileUnboundScript / CompileModule
//   - Context -> Global / NewObject / RunScript
//   - Value -> Release
//   - Module / UnboundScript -> Release
//
// Convenience helpers remain available, but they are secondary to the explicit
// APIs and should not be the foundation of a long-lived host runtime design.
package gv8
