package gv8

// #include <stdlib.h>
// #include "gv8.h"
import "C"

import (
	"fmt"
	"sync"
	"unsafe"
)

var (
	ctxMu   sync.RWMutex
	ctxSeq  int
	ctxRefs = map[int]*Context{}
)

type Context struct {
	ref int
	ptr C.GV8ContextPtr
	iso *Isolate

	moduleResolver ModuleResolver
}

func NewContext(iso *Isolate) *Context {
	if iso == nil {
		panic("gv8: nil isolate")
	}

	ctxMu.Lock()
	ctxSeq++
	ref := ctxSeq
	ctxMu.Unlock()

	ctx := &Context{
		ref: ref,
		ptr: C.GV8NewContext(iso.ptr, C.int(ref)),
		iso: iso,
	}
	setContext(ctx)
	return ctx
}

func setContext(ctx *Context) {
	ctxMu.Lock()
	defer ctxMu.Unlock()
	ctxRefs[ctx.ref] = ctx
}

func deleteContext(ref int) {
	ctxMu.Lock()
	defer ctxMu.Unlock()
	delete(ctxRefs, ref)
}

func getContext(ref int) *Context {
	ctxMu.RLock()
	defer ctxMu.RUnlock()
	return ctxRefs[ref]
}

func (c *Context) Close() {
	if c == nil || c.ptr == nil {
		return
	}
	deleteContext(c.ref)
	C.GV8ContextDispose(c.ptr)
	c.ptr = nil
}

func (c *Context) Isolate() *Isolate {
	return c.iso
}

func (c *Context) Global() *Object {
	return &Object{Value: newValue(c, C.GV8ContextGlobal(c.ptr))}
}

func (c *Context) GetGlobal(name string) (*Value, error) {
	global := c.Global()
	defer global.Release()
	return global.Get(name)
}

func (c *Context) SetGlobal(name string, value any) error {
	global := c.Global()
	defer global.Release()
	return global.Set(name, value)
}

func (c *Context) SetGlobals(values map[string]any) error {
	for name, value := range values {
		if err := c.SetGlobal(name, value); err != nil {
			return err
		}
	}
	return nil
}

func (c *Context) Bind(name string, fn HostFunction) error {
	hostFn, err := NewFunction(c, fn)
	if err != nil {
		return err
	}
	defer hostFn.Release()
	return c.SetGlobal(name, hostFn)
}

func (c *Context) NewObject() (*Object, error) {
	rtn := C.GV8ContextNewObject(c.ptr)
	value, err := valueResult(c, rtn)
	if err != nil {
		return nil, err
	}
	return value.Object(), nil
}

func (c *Context) RunScript(source string, origin string) (*Value, error) {
	if c == nil {
		return nil, fmt.Errorf("gv8: nil context")
	}

	csource := C.CString(source)
	corigin := C.CString(origin)
	defer C.free(unsafe.Pointer(csource))
	defer C.free(unsafe.Pointer(corigin))

	rtn := C.GV8ContextRunScript(c.ptr, csource, corigin, 0, 0)
	return valueResult(c, rtn)
}
