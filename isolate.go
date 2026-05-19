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
	"context"
	"fmt"
	"sync"
)

type IsolateOptions struct {
	InitialHeapSizeBytes uint64
	MaxHeapSizeBytes     uint64
}

type MemoryPressureLevel int

const (
	MemoryPressureNone MemoryPressureLevel = iota
	MemoryPressureModerate
	MemoryPressureCritical
)

type HeapStatistics struct {
	TotalHeapSize           uint64
	TotalHeapSizeExecutable uint64
	TotalPhysicalSize       uint64
	TotalAvailableSize      uint64
	TotalGlobalHandlesSize  uint64
	UsedGlobalHandlesSize   uint64
	UsedHeapSize            uint64
	HeapSizeLimit           uint64
	MallocedMemory          uint64
	ExternalMemory          uint64
	PeakMallocedMemory      uint64
	NativeContexts          uint64
	DetachedContexts        uint64
	TotalAllocatedBytes     uint64
}

type Isolate struct {
	ptr C.GV8IsolatePtr

	mu          sync.Mutex
	ownerThread uintptr
	depth       int
}

func NewIsolate() *Isolate {
	return NewIsolateWithOptions(IsolateOptions{})
}

func NewIsolateWithOptions(options IsolateOptions) *Isolate {
	initialize()
	if options.InitialHeapSizeBytes == 0 && options.MaxHeapSizeBytes == 0 {
		return &Isolate{ptr: C.GV8NewIsolate()}
	}
	return &Isolate{
		ptr: C.GV8NewIsolateWithHeapLimit(
			C.uint64_t(options.InitialHeapSizeBytes),
			C.uint64_t(options.MaxHeapSizeBytes),
		),
	}
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

func (i *Isolate) TerminateExecution() {
	if i == nil || i.ptr == nil {
		return
	}
	C.GV8IsolateTerminateExecution(i.ptr)
}

func (i *Isolate) IsExecutionTerminating() bool {
	if i == nil || i.ptr == nil {
		return false
	}
	return C.GV8IsolateIsExecutionTerminating(i.ptr) != 0
}

func (i *Isolate) CancelTerminateExecution() {
	if i == nil || i.ptr == nil {
		return
	}
	C.GV8IsolateCancelTerminateExecution(i.ptr)
}

func (i *Isolate) LowMemoryNotification() {
	if i == nil || i.ptr == nil {
		return
	}
	C.GV8IsolateLowMemoryNotification(i.ptr)
}

func (i *Isolate) MemoryPressureNotification(level MemoryPressureLevel) {
	if i == nil || i.ptr == nil {
		return
	}
	C.GV8IsolateMemoryPressureNotification(i.ptr, C.int(level))
}

func (i *Isolate) HeapStatistics() HeapStatistics {
	if i == nil || i.ptr == nil {
		return HeapStatistics{}
	}
	stats := C.GV8IsolateGetHeapStatistics(i.ptr)
	return HeapStatistics{
		TotalHeapSize:           uint64(stats.total_heap_size),
		TotalHeapSizeExecutable: uint64(stats.total_heap_size_executable),
		TotalPhysicalSize:       uint64(stats.total_physical_size),
		TotalAvailableSize:      uint64(stats.total_available_size),
		TotalGlobalHandlesSize:  uint64(stats.total_global_handles_size),
		UsedGlobalHandlesSize:   uint64(stats.used_global_handles_size),
		UsedHeapSize:            uint64(stats.used_heap_size),
		HeapSizeLimit:           uint64(stats.heap_size_limit),
		MallocedMemory:          uint64(stats.malloced_memory),
		ExternalMemory:          uint64(stats.external_memory),
		PeakMallocedMemory:      uint64(stats.peak_malloced_memory),
		NativeContexts:          uint64(stats.number_of_native_contexts),
		DetachedContexts:        uint64(stats.number_of_detached_contexts),
		TotalAllocatedBytes:     uint64(stats.total_allocated_bytes),
	}
}

func (i *Isolate) TerminateOnContextDone(ctx context.Context) func() {
	if i == nil || i.ptr == nil || ctx == nil {
		return func() {}
	}

	stop := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			i.TerminateExecution()
		case <-stop:
		}
	}()
	return func() {
		select {
		case <-stop:
		default:
			close(stop)
		}
	}
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
