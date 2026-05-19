package gv8

// #include <stdlib.h>
// #include "gv8.h"
import "C"

import "unsafe"

// UnboundScript is a compiled script that can be run in a context belonging to
// the same isolate.
type UnboundScript struct {
	ptr C.GV8UnboundScriptPtr
	iso *Isolate
}

func compileUnboundScript(iso *Isolate, source string, origin ScriptOrigin) (*UnboundScript, error) {
	release, err := iso.enter()
	if err != nil {
		return nil, err
	}
	defer release()

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
		return nil, observeJSError(iso, newJSError(rtn.error))
	}
	return &UnboundScript{ptr: rtn.ptr, iso: iso}, nil
}

// Release frees the underlying compiled script handle.
func (s *UnboundScript) Release() {
	if s == nil || s.ptr == nil {
		return
	}
	release := s.iso.mustEnter()
	defer release()
	C.GV8UnboundScriptRelease(s.ptr)
	s.ptr = nil
}

// Run binds the script to ctx and executes it.
func (s *UnboundScript) Run(ctx *Context) (*Value, error) {
	if s == nil || s.ptr == nil {
		return nil, &JSError{Message: "gv8: script is no longer valid"}
	}
	if err := ctx.ensureOpen(); err != nil {
		return nil, err
	}
	release, err := ctx.iso.enter()
	if err != nil {
		return nil, err
	}
	defer release()
	rtn := C.GV8UnboundScriptRun(ctx.ptr, s.ptr)
	return valueResult(ctx, rtn)
}
