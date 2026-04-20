//go:build linux && amd64

package gv8

// #cgo LDFLAGS: -L${SRCDIR}/internal/v8/linux_x86_64 -lgv8 -Wl,-rpath,${SRCDIR}/internal/v8/linux_x86_64
import "C"
