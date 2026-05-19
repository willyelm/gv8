package gv8

// #include <pthread.h>
// #include <stdint.h>
// #include "gv8.h"
//
// static uintptr_t GV8CurrentThreadID() {
//   return (uintptr_t)pthread_self();
// }
import "C"

import (
	"fmt"
	"sync"
)

type Isolate struct {
	ptr C.GV8IsolatePtr

	mu          sync.Mutex
	ownerThread uintptr
	depth       int
}

func NewIsolate() *Isolate {
	initialize()
	return &Isolate{ptr: C.GV8NewIsolate()}
}

func (i *Isolate) Dispose() {
	if i == nil || i.ptr == nil {
		return
	}
	release := i.mustEnter()
	defer release()
	C.GV8IsolateDispose(i.ptr)
	i.ptr = nil
}

func (i *Isolate) PerformMicrotaskCheckpoint() {
	if i == nil || i.ptr == nil {
		return
	}
	release := i.mustEnter()
	defer release()
	C.GV8IsolatePerformMicrotaskCheckpoint(i.ptr)
}

func (i *Isolate) CompileUnboundScript(source string, origin ScriptOrigin) (*UnboundScript, error) {
	return compileUnboundScript(i, source, origin)
}

func (i *Isolate) CompileModule(source string, origin ScriptOrigin) (*Module, error) {
	return compileModule(i, source, origin)
}

func (i *Isolate) enter() (func(), error) {
	if i == nil || i.ptr == nil {
		return nil, fmt.Errorf("gv8: nil isolate")
	}

	threadID := uintptr(C.GV8CurrentThreadID())

	i.mu.Lock()
	defer i.mu.Unlock()

	if i.depth == 0 {
		i.ownerThread = threadID
		i.depth = 1
		return i.leave, nil
	}
	if i.ownerThread != threadID {
		return nil, fmt.Errorf("gv8: isolate is already in use; callers must externally synchronize access")
	}

	i.depth++
	return i.leave, nil
}

func (i *Isolate) mustEnter() func() {
	release, err := i.enter()
	if err != nil {
		panic(err)
	}
	return release
}

func (i *Isolate) leave() {
	threadID := uintptr(C.GV8CurrentThreadID())

	i.mu.Lock()
	defer i.mu.Unlock()

	if i.depth == 0 || i.ownerThread != threadID {
		panic("gv8: isolate access released from non-owner thread")
	}

	i.depth--
	if i.depth == 0 {
		i.ownerThread = 0
	}
}
