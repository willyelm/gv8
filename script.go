package gv8

// #include <stdlib.h>
// #include "gv8.h"
import "C"

import "unsafe"

type UnboundScript struct {
	ptr C.GV8UnboundScriptPtr
	iso *Isolate
}

func compileUnboundScript(iso *Isolate, source string, origin ScriptOrigin) (*UnboundScript, error) {
	csource := C.CString(source)
	corigin := C.CString(origin.ResourceName)
	defer C.free(unsafe.Pointer(csource))
	defer C.free(unsafe.Pointer(corigin))

	rtn := C.GV8CompileUnboundScript(
		iso.ptr,
		csource,
		corigin,
		C.int(origin.LineOffset),
		C.int(origin.ColumnOffset),
	)
	if rtn.ptr == nil {
		return nil, newJSError(rtn.error)
	}
	return &UnboundScript{ptr: rtn.ptr, iso: iso}, nil
}

func (s *UnboundScript) Release() {
	if s == nil || s.ptr == nil {
		return
	}
	C.GV8UnboundScriptRelease(s.ptr)
	s.ptr = nil
}

func (s *UnboundScript) Run(ctx *Context) (*Value, error) {
	if s == nil || s.ptr == nil {
		return nil, &JSError{Message: "gv8: script is no longer valid"}
	}
	if err := ctx.ensureOpen(); err != nil {
		return nil, err
	}
	rtn := C.GV8UnboundScriptRun(ctx.ptr, s.ptr)
	return valueResult(ctx, rtn)
}
