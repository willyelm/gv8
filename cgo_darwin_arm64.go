//go:build darwin && arm64

package gv8

// #cgo LDFLAGS: -L${SRCDIR}/internal/v8/darwin_arm64 -lgv8 -Wl,-rpath,${SRCDIR}/internal/v8/darwin_arm64
import "C"
