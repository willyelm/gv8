package gv8

// #include <stdlib.h>
// #include "gv8.h"
import "C"

import "unsafe"

type Module struct {
	ptr C.GV8ModulePtr
	iso *Isolate
}

type ModuleResolver interface {
	ResolveModule(ctx *Context, specifier string, referrer *Module) (*Module, error)
}

func compileModule(iso *Isolate, source string, origin ScriptOrigin) (*Module, error) {
	csource := C.CString(source)
	corigin := C.CString(origin.ResourceName)
	defer C.free(unsafe.Pointer(csource))
	defer C.free(unsafe.Pointer(corigin))

	rtn := C.GV8CompileModule(
		iso.ptr,
		csource,
		corigin,
		C.int(origin.LineOffset),
		C.int(origin.ColumnOffset),
	)
	if rtn.ptr == nil {
		return nil, newJSError(rtn.error)
	}
	return &Module{ptr: rtn.ptr, iso: iso}, nil
}

func (m *Module) Instantiate(ctx *Context, resolver ModuleResolver) error {
	ctx.moduleResolver = resolver
	rtn := C.GV8ModuleInstantiate(ctx.ptr, m.ptr)
	if rtn.msg == nil {
		return nil
	}
	return newJSError(rtn)
}

func (m *Module) Evaluate(ctx *Context) (*Value, error) {
	rtn := C.GV8ModuleEvaluate(ctx.ptr, m.ptr)
	return valueResult(ctx, rtn)
}

func (m *Module) Namespace(ctx *Context) *Object {
	return &Object{Value: newValue(ctx, C.GV8ModuleGetNamespace(ctx.ptr, m.ptr))}
}

//export gv8ResolveModuleCallback
func gv8ResolveModuleCallback(ctxRef C.int, spec *C.char, referrer C.GV8ModulePtr) C.GV8ResolvedModule {
	ctx := getContext(int(ctxRef))
	if ctx == nil || ctx.moduleResolver == nil {
		msg, _ := newErrorValue(ctx, "gv8: no module resolver configured")
		if msg != nil {
			return C.GV8ResolvedModule{error_value: msg.ptr}
		}
		return C.GV8ResolvedModule{}
	}

	mod, err := ctx.moduleResolver.ResolveModule(ctx, C.GoString(spec), &Module{
		ptr: referrer,
		iso: ctx.iso,
	})
	if err == nil {
		return C.GV8ResolvedModule{module: mod.ptr}
	}

	msg, msgErr := newErrorValue(ctx, "cannot resolve module: "+err.Error())
	if msgErr != nil {
		return C.GV8ResolvedModule{}
	}
	return C.GV8ResolvedModule{error_value: msg.ptr}
}
