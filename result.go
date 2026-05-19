package gv8

// #include <stdlib.h>
// #include "gv8.h"
import "C"

import "unsafe"

func valueResult(ctx *Context, rtn C.GV8RtnValue) (*Value, error) {
	if rtn.value == nil {
		err := newJSError(rtn.error)
		if ctx != nil && ctx.iso != nil {
			observeJSError(ctx.iso, err)
		}
		return nil, err
	}
	return newValue(ctx, rtn.value), nil
}

func newJSError(rtn C.GV8RtnError) error {
	if rtn.msg == nil {
		return nil
	}
	err := &JSError{
		Message: C.GoString(rtn.msg),
	}
	if rtn.location != nil {
		err.Location = C.GoString(rtn.location)
	}
	if rtn.stack != nil {
		err.Stack = C.GoString(rtn.stack)
	}
	C.free(unsafe.Pointer(rtn.msg))
	C.free(unsafe.Pointer(rtn.location))
	C.free(unsafe.Pointer(rtn.stack))
	return err
}

func observeJSError(iso *Isolate, err error) error {
	if iso == nil || err == nil {
		return err
	}
	if jsErr, ok := err.(*JSError); ok {
		iso.noteException(jsErr)
	}
	return err
}
