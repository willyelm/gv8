package gv8

// #include <stdlib.h>
// #include "gv8.h"
import "C"

import "unsafe"

func NewStringValue(ctx *Context, value string) (*Value, error) {
	cvalue := C.CString(value)
	defer C.free(unsafe.Pointer(cvalue))

	rtn := C.GV8NewString(ctx.ptr, cvalue, C.int(len(value)))
	return valueResult(ctx, rtn)
}

func newErrorValue(ctx *Context, value string) (*Value, error) {
	if ctx == nil {
		return nil, nil
	}
	return NewStringValue(ctx, value)
}
