package gv8

// #include <stdlib.h>
// #include "gv8.h"
import "C"

import "unsafe"

// Snapshot is V8 startup data that can be used to create a new isolate.
type Snapshot []byte

// SnapshotBuilder creates V8 startup snapshots from trusted JavaScript source.
type SnapshotBuilder struct {
	ptr C.GV8SnapshotBuilderPtr
}

// NewSnapshotBuilder creates a builder for a single V8 snapshot.
func NewSnapshotBuilder() *SnapshotBuilder {
	initialize()
	return &SnapshotBuilder{ptr: C.GV8NewSnapshotBuilder()}
}

// RunScript executes source in the snapshot builder's startup context.
func (b *SnapshotBuilder) RunScript(source string, origin ScriptOrigin) error {
	if b == nil || b.ptr == nil {
		return invalidSnapshotBuilderError()
	}
	csource := C.CString(source)
	corigin := C.CString(origin.ResourceName)
	defer C.free(unsafe.Pointer(csource))
	defer C.free(unsafe.Pointer(corigin))

	return newJSError(C.GV8SnapshotBuilderRunScript(
		b.ptr,
		csource,
		corigin,
		C.int(origin.LineOffset),
		C.int(origin.ColumnOffset),
	))
}

// Build finalizes and returns the snapshot data.
func (b *SnapshotBuilder) Build() (Snapshot, error) {
	if b == nil || b.ptr == nil {
		return nil, invalidSnapshotBuilderError()
	}
	rtn := C.GV8SnapshotBuilderBuild(b.ptr)
	if rtn.error.msg != nil {
		return nil, newJSError(rtn.error)
	}
	if rtn.data == nil || rtn.length == 0 {
		return nil, nil
	}
	defer C.free(unsafe.Pointer(rtn.data))
	return C.GoBytes(unsafe.Pointer(rtn.data), C.int(rtn.length)), nil
}

// Release frees the builder without producing a snapshot.
func (b *SnapshotBuilder) Release() {
	if b == nil || b.ptr == nil {
		return
	}
	C.GV8SnapshotBuilderRelease(b.ptr)
	b.ptr = nil
}
