package gv8

//go:generate clang-format -i --verbose -style=Chromium gv8.h internal/gv8.cc

// #include "gv8.h"
// #cgo CFLAGS: -I${SRCDIR}/internal/v8/include
import "C"

import (
	"fmt"
	"sync"
)

var initOnce sync.Once

func initialize() {
	initOnce.Do(func() {
		const want = int(C.GV8_ABI_VERSION)
		got := int(C.GV8ABIVersion())
		if got != want {
			panic(fmt.Sprintf(
				"gv8: bundled runtime ABI mismatch: header expects version %d, runtime reports version %d — rebuild with 'make build'",
				want, got,
			))
		}
		C.GV8Init()
	})
}
