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

// NewNumberValue creates a JavaScript Number in ctx.
func NewNumberValue(ctx *Context, value float64) (*Value, error) {
	if err := ctx.ensureOpen(); err != nil {
		return nil, err
	}
	release, err := ctx.iso.enter()
	if err != nil {
		return nil, err
	}
	defer release()
	rtn := C.GV8NewNumber(ctx.ptr, C.double(value))
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

// NewArrayBufferCopy creates an ArrayBuffer by copying data into V8.
func NewArrayBufferCopy(ctx *Context, data []byte) (*Value, error) {
	if err := ctx.ensureOpen(); err != nil {
		return nil, err
	}
	release, err := ctx.iso.enter()
	if err != nil {
		return nil, err
	}
	defer release()
	ptr := bytesPtr(data)
	rtn := C.GV8NewArrayBufferCopy(ctx.ptr, ptr, C.int(len(data)))
	return valueResult(ctx, rtn)
}

// NewUint8ArrayCopy creates a Uint8Array by copying data into V8.
func NewUint8ArrayCopy(ctx *Context, data []byte) (*Value, error) {
	if err := ctx.ensureOpen(); err != nil {
		return nil, err
	}
	release, err := ctx.iso.enter()
	if err != nil {
		return nil, err
	}
	defer release()
	ptr := bytesPtr(data)
	rtn := C.GV8NewUint8ArrayCopy(ctx.ptr, ptr, C.int(len(data)))
	return valueResult(ctx, rtn)
}

func bytesPtr(data []byte) *C.uint8_t {
	if len(data) == 0 {
		return nil
	}
	return (*C.uint8_t)(unsafe.Pointer(&data[0]))
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
