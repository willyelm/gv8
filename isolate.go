package gv8

// #include "gv8.h"
import "C"

type Isolate struct {
	ptr C.GV8IsolatePtr
}

func NewIsolate() *Isolate {
	initialize()
	return &Isolate{ptr: C.GV8NewIsolate()}
}

func (i *Isolate) Dispose() {
	if i == nil || i.ptr == nil {
		return
	}
	C.GV8IsolateDispose(i.ptr)
	i.ptr = nil
}

func (i *Isolate) PerformMicrotaskCheckpoint() {
	if i == nil || i.ptr == nil {
		return
	}
	C.GV8IsolatePerformMicrotaskCheckpoint(i.ptr)
}

func (i *Isolate) CompileUnboundScript(source string, origin ScriptOrigin) (*UnboundScript, error) {
	return compileUnboundScript(i, source, origin)
}

func (i *Isolate) CompileModule(source string, origin ScriptOrigin) (*Module, error) {
	return compileModule(i, source, origin)
}
