package gv8

// #include <stdlib.h>
// #include "gv8.h"
import "C"

import "unsafe"

type Value struct {
	ptr C.GV8ValuePtr
	ctx *Context
}

func newValue(ctx *Context, ptr C.GV8ValuePtr) *Value {
	if ptr == nil {
		return nil
	}
	return &Value{ptr: ptr, ctx: ctx}
}

func (v *Value) Release() {
	if v == nil || v.ptr == nil {
		return
	}
	C.GV8ValueRelease(v.ptr)
	v.ptr = nil
}

func (v *Value) String() string {
	rtn := C.GV8ValueToString(v.ptr)
	if rtn.error.msg != nil {
		panic(newJSError(rtn.error))
	}
	defer C.free(unsafe.Pointer(rtn.data))
	return C.GoStringN(rtn.data, rtn.length)
}

func (v *Value) Integer() int64 {
	return int64(C.GV8ValueToInteger(v.ptr))
}

type Object struct {
	*Value
}

func (o *Object) Get(name string) (*Value, error) {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))

	rtn := C.GV8ObjectGet(o.ptr, cname)
	return valueResult(o.ctx, rtn)
}
