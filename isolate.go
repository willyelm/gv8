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
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"
)

type IsolateOptions struct {
	InitialHeapSizeBytes uint64
	MaxHeapSizeBytes     uint64
	Snapshot             Snapshot
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

type PromiseRejectEvent int

const (
	PromiseRejectWithNoHandler PromiseRejectEvent = iota
	PromiseHandlerAddedAfterReject
	PromiseRejectAfterResolved
	PromiseResolveAfterResolved
)

type PromiseRejectInfo struct {
	Event PromiseRejectEvent
	Error *JSError
}

type ObservabilitySnapshot struct {
	UnhandledPromiseRejections uint64
	Exceptions                 uint64
	ModuleResolutionFailures   uint64
}

type UnhandledPromiseRejectionHandler func(PromiseRejectInfo)
type ExceptionHandler func(*JSError)
type ModuleResolutionFailureHandler func(specifier string, referrer string, err error)

var (
	isolateRefsMu  sync.RWMutex
	isolateRefsSeq int
	isolateRefs    = map[int]*Isolate{}
)

// Isolate is the root ownership boundary for all V8 state created through gv8.
type Isolate struct {
	ptr C.GV8IsolatePtr
	ref int

	mu          sync.Mutex
	ownerThread uintptr
	depth       int

	terminating atomic.Bool

	obMu                          sync.RWMutex
	unhandledPromiseRejectionHook UnhandledPromiseRejectionHandler
	exceptionHook                 ExceptionHandler
	moduleResolutionFailureHook   ModuleResolutionFailureHandler
	unhandledPromiseRejections    atomic.Uint64
	exceptions                    atomic.Uint64
	moduleResolutionFailures      atomic.Uint64
}

// NewIsolate creates an isolate with default heap settings.
func NewIsolate() *Isolate {
	return NewIsolateWithOptions(IsolateOptions{})
}

// NewIsolateWithOptions creates an isolate with explicit heap options.
func NewIsolateWithOptions(options IsolateOptions) *Isolate {
	initialize()
	ref := nextIsolateRef()
	iso := &Isolate{ref: ref}
	if len(options.Snapshot) > 0 {
		iso.ptr = C.GV8NewIsolateWithSnapshot(
			(*C.uint8_t)(unsafe.Pointer(&options.Snapshot[0])),
			C.int(len(options.Snapshot)),
			C.uint64_t(options.InitialHeapSizeBytes),
			C.uint64_t(options.MaxHeapSizeBytes),
		)
		if iso.ptr == nil {
			panic("gv8: invalid snapshot data")
		}
	} else if options.InitialHeapSizeBytes == 0 && options.MaxHeapSizeBytes == 0 {
		iso.ptr = C.GV8NewIsolate()
	} else {
		iso.ptr = C.GV8NewIsolateWithHeapLimit(
			C.uint64_t(options.InitialHeapSizeBytes),
			C.uint64_t(options.MaxHeapSizeBytes),
		)
	}
	setIsolateRef(iso)
	C.GV8IsolateSetObserverRef(iso.ptr, C.int(ref))
	return iso
}

func (i *Isolate) Dispose() {
	if i == nil || i.ptr == nil {
		return
	}
	release := i.mustEnter()
	defer release()
	deleteIsolateRef(i.ref)
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
	i.terminating.Store(true)
	C.GV8IsolateTerminateExecution(i.ptr)
}

func (i *Isolate) IsExecutionTerminating() bool {
	if i == nil || i.ptr == nil {
		return false
	}
	return i.terminating.Load()
}

func (i *Isolate) CancelTerminateExecution() {
	if i == nil || i.ptr == nil {
		return
	}
	i.terminating.Store(false)
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

func (i *Isolate) ObservabilitySnapshot() ObservabilitySnapshot {
	if i == nil {
		return ObservabilitySnapshot{}
	}
	return ObservabilitySnapshot{
		UnhandledPromiseRejections: i.unhandledPromiseRejections.Load(),
		Exceptions:                 i.exceptions.Load(),
		ModuleResolutionFailures:   i.moduleResolutionFailures.Load(),
	}
}

func (i *Isolate) SetUnhandledPromiseRejectionHandler(handler UnhandledPromiseRejectionHandler) {
	if i == nil {
		return
	}
	i.obMu.Lock()
	defer i.obMu.Unlock()
	i.unhandledPromiseRejectionHook = handler
}

func (i *Isolate) SetExceptionHandler(handler ExceptionHandler) {
	if i == nil {
		return
	}
	i.obMu.Lock()
	defer i.obMu.Unlock()
	i.exceptionHook = handler
}

func (i *Isolate) SetModuleResolutionFailureHandler(handler ModuleResolutionFailureHandler) {
	if i == nil {
		return
	}
	i.obMu.Lock()
	defer i.obMu.Unlock()
	i.moduleResolutionFailureHook = handler
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

	runtime.LockOSThread()
	threadID := uintptr(C.GV8CurrentThreadID())

	i.mu.Lock()
	defer i.mu.Unlock()

	if i.depth == 0 {
		i.ownerThread = threadID
		i.depth = 1
		return i.leave, nil
	}
	if i.ownerThread != threadID {
		runtime.UnlockOSThread()
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
	runtime.UnlockOSThread()
}

func (i *Isolate) noteUnhandledPromiseRejection(event PromiseRejectEvent, err *JSError) {
	if i == nil {
		return
	}
	i.unhandledPromiseRejections.Add(1)
	i.obMu.RLock()
	handler := i.unhandledPromiseRejectionHook
	i.obMu.RUnlock()
	if handler != nil {
		handler(PromiseRejectInfo{Event: event, Error: err})
	}
}

func (i *Isolate) noteException(err *JSError) {
	if i == nil {
		return
	}
	i.exceptions.Add(1)
	i.obMu.RLock()
	handler := i.exceptionHook
	i.obMu.RUnlock()
	if handler != nil {
		handler(err)
	}
}

func (i *Isolate) noteModuleResolutionFailure(specifier string, referrer string, err error) {
	if i == nil {
		return
	}
	i.moduleResolutionFailures.Add(1)
	i.obMu.RLock()
	handler := i.moduleResolutionFailureHook
	i.obMu.RUnlock()
	if handler != nil {
		handler(specifier, referrer, err)
	}
}

func nextIsolateRef() int {
	isolateRefsMu.Lock()
	defer isolateRefsMu.Unlock()
	isolateRefsSeq++
	return isolateRefsSeq
}

func setIsolateRef(iso *Isolate) {
	isolateRefsMu.Lock()
	defer isolateRefsMu.Unlock()
	isolateRefs[iso.ref] = iso
}

func deleteIsolateRef(ref int) {
	isolateRefsMu.Lock()
	defer isolateRefsMu.Unlock()
	delete(isolateRefs, ref)
}

func getIsolateRef(ref int) *Isolate {
	isolateRefsMu.RLock()
	defer isolateRefsMu.RUnlock()
	return isolateRefs[ref]
}

//export gv8PromiseRejectCallback
func gv8PromiseRejectCallback(isolateRef C.int, event C.int, errorInfo C.GV8RtnError) {
	iso := getIsolateRef(int(isolateRef))
	if iso == nil {
		_ = newJSError(errorInfo)
		return
	}
	var jsErr *JSError
	if err := newJSError(errorInfo); err != nil {
		jsErr, _ = err.(*JSError)
	}
	iso.noteUnhandledPromiseRejection(PromiseRejectEvent(event), jsErr)
}
