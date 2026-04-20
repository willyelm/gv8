package gv8

// #include "gv8.h"
import "C"

import (
	"sync"
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
