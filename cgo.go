package gv8

//go:generate clang-format -i --verbose -style=Chromium gv8.h internal/gv8.cc

// #include "gv8.h"
// #cgo CFLAGS: -I${SRCDIR}/internal/v8/include
import "C"

import "sync"

var initOnce sync.Once

func initialize() {
	initOnce.Do(func() {
		C.GV8Init()
	})
}
