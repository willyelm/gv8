package gv8

// #include <stdlib.h>
// #include "gv8.h"
import "C"

import "unsafe"

func NewStringValue(ctx *Context, value string) (*Value, error) {
	if err := ctx.ensureOpen(); err != nil {
		return nil, err
	}
	release, err := ctx.iso.enter()
	if err != nil {
		return nil, err
	}
	defer release()
	cvalue := C.CString(value)
	defer C.free(unsafe.Pointer(cvalue))

	rtn := C.GV8NewString(ctx.ptr, cvalue, C.int(len(value)))
	return valueResult(ctx, rtn)
}

func NewBooleanValue(ctx *Context, value bool) (*Value, error) {
	if err := ctx.ensureOpen(); err != nil {
		return nil, err
	}
	release, err := ctx.iso.enter()
	if err != nil {
		return nil, err
	}
	defer release()
	flag := 0
	if value {
		flag = 1
	}
	rtn := C.GV8NewBoolean(ctx.ptr, C.int(flag))
	return valueResult(ctx, rtn)
}

func NewIntegerValue(ctx *Context, value int64) (*Value, error) {
	if err := ctx.ensureOpen(); err != nil {
		return nil, err
	}
	release, err := ctx.iso.enter()
	if err != nil {
		return nil, err
	}
	defer release()
	rtn := C.GV8NewInteger(ctx.ptr, C.int64_t(value))
	return valueResult(ctx, rtn)
}

func NewNullValue(ctx *Context) (*Value, error) {
	if err := ctx.ensureOpen(); err != nil {
		return nil, err
	}
	release, err := ctx.iso.enter()
	if err != nil {
		return nil, err
	}
	defer release()
	rtn := C.GV8NewNull(ctx.ptr)
	return valueResult(ctx, rtn)
}

func NewUndefinedValue(ctx *Context) (*Value, error) {
	if err := ctx.ensureOpen(); err != nil {
		return nil, err
	}
	release, err := ctx.iso.enter()
	if err != nil {
		return nil, err
	}
	defer release()
	rtn := C.GV8NewUndefined(ctx.ptr)
	return valueResult(ctx, rtn)
}

func newErrorValue(ctx *Context, value string) (*Value, error) {
	if ctx == nil {
		return nil, nil
	}
	release, err := ctx.iso.enter()
	if err != nil {
		return nil, err
	}
	defer release()
	cvalue := C.CString(value)
	defer C.free(unsafe.Pointer(cvalue))
	rtn := C.GV8NewError(ctx.ptr, cvalue, C.int(len(value)))
	return valueResult(ctx, rtn)
}
