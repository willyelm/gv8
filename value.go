package gv8

// #include <stdlib.h>
// #include "gv8.h"
import "C"

import "unsafe"

// Value is the base wrapper for V8 values that belong to a Context.
type Value struct {
	ptr      C.GV8ValuePtr
	ctx      *Context
	borrowed bool
}

func newValue(ctx *Context, ptr C.GV8ValuePtr) *Value {
	if ptr == nil {
		return nil
	}
	value := &Value{ptr: ptr, ctx: ctx}
	if ctx != nil {
		ctx.trackValue(value)
	}
	return value
}

func newBorrowedValue(ctx *Context, ptr C.GV8ValuePtr) *Value {
	if ptr == nil {
		return nil
	}
	return &Value{ptr: ptr, ctx: ctx, borrowed: true}
}

func (v *Value) invalidate() {
	if v == nil {
		return
	}
	v.ptr = nil
}

func (v *Value) valid() bool {
	return v != nil && v.ptr != nil && (v.ctx == nil || !v.ctx.isClosed())
}

func (v *Value) Release() {
	if v == nil || v.ptr == nil {
		return
	}
	if v.borrowed {
		v.ptr = nil
		return
	}
	if v.ctx != nil {
		v.ctx.untrackValue(v)
		release := v.ctx.iso.mustEnter()
		defer release()
	}
	C.GV8ValueRelease(v.ptr)
	v.ptr = nil
}

// StringValue converts the value to a string and reports conversion errors.
func (v *Value) StringValue() (string, error) {
	if !v.valid() {
		return "", invalidValueError()
	}
	release := v.ctx.iso.mustEnter()
	defer release()
	rtn := C.GV8ValueToString(v.ptr)
	if rtn.error.msg != nil {
		return "", newJSError(rtn.error)
	}
	defer C.free(unsafe.Pointer(rtn.data))
	return C.GoStringN(rtn.data, rtn.length), nil
}

func (v *Value) Integer() int64 {
	if !v.valid() {
		return 0
	}
	release := v.ctx.iso.mustEnter()
	defer release()
	return int64(C.GV8ValueToInteger(v.ptr))
}

func (v *Value) Boolean() bool {
	if !v.valid() {
		return false
	}
	release := v.ctx.iso.mustEnter()
	defer release()
	return C.GV8ValueToBoolean(v.ptr) != 0
}

func (v *Value) IsObject() bool {
	if !v.valid() {
		return false
	}
	release := v.ctx.iso.mustEnter()
	defer release()
	return C.GV8ValueIsObject(v.ptr) != 0
}

func (v *Value) IsString() bool {
	if !v.valid() {
		return false
	}
	release := v.ctx.iso.mustEnter()
	defer release()
	return C.GV8ValueIsString(v.ptr) != 0
}

func (v *Value) IsBoolean() bool {
	if !v.valid() {
		return false
	}
	release := v.ctx.iso.mustEnter()
	defer release()
	return C.GV8ValueIsBoolean(v.ptr) != 0
}

func (v *Value) IsUndefined() bool {
	if !v.valid() {
		return false
	}
	release := v.ctx.iso.mustEnter()
	defer release()
	return C.GV8ValueIsUndefined(v.ptr) != 0
}

func (v *Value) IsNull() bool {
	if !v.valid() {
		return false
	}
	release := v.ctx.iso.mustEnter()
	defer release()
	return C.GV8ValueIsNull(v.ptr) != 0
}

func (v *Value) IsPromise() bool {
	if !v.valid() {
		return false
	}
	release := v.ctx.iso.mustEnter()
	defer release()
	return C.GV8ValueIsPromise(v.ptr) != 0
}

func (v *Value) IsFunction() bool {
	if !v.valid() {
		return false
	}
	release := v.ctx.iso.mustEnter()
	defer release()
	return C.GV8ValueIsFunction(v.ptr) != 0
}

func (v *Value) IsArrayBuffer() bool {
	if !v.valid() {
		return false
	}
	release := v.ctx.iso.mustEnter()
	defer release()
	return C.GV8ValueIsArrayBuffer(v.ptr) != 0
}

func (v *Value) IsArrayBufferView() bool {
	if !v.valid() {
		return false
	}
	release := v.ctx.iso.mustEnter()
	defer release()
	return C.GV8ValueIsArrayBufferView(v.ptr) != 0
}

func (v *Value) IsUint8Array() bool {
	if !v.valid() {
		return false
	}
	release := v.ctx.iso.mustEnter()
	defer release()
	return C.GV8ValueIsUint8Array(v.ptr) != 0
}

func (v *Value) IsInt8Array() bool {
	if !v.valid() {
		return false
	}
	release := v.ctx.iso.mustEnter()
	defer release()
	return C.GV8ValueIsInt8Array(v.ptr) != 0
}

func (v *Value) IsUint8ClampedArray() bool {
	if !v.valid() {
		return false
	}
	release := v.ctx.iso.mustEnter()
	defer release()
	return C.GV8ValueIsUint8ClampedArray(v.ptr) != 0
}

// Bytes copies byte data out of ArrayBuffer and ArrayBufferView values.
func (v *Value) Bytes() ([]byte, error) {
	if !v.valid() {
		return nil, invalidValueError()
	}
	release, err := v.ctx.iso.enter()
	if err != nil {
		return nil, err
	}
	defer release()
	rtn := C.GV8ValueBytes(v.ptr)
	if rtn.error.msg != nil {
		return nil, newJSError(rtn.error)
	}
	if rtn.data == nil || rtn.length == 0 {
		return nil, nil
	}
	defer C.free(unsafe.Pointer(rtn.data))
	return C.GoBytes(unsafe.Pointer(rtn.data), C.int(rtn.length)), nil
}

// Len returns the JavaScript length for array-like values.
func (v *Value) Len() uint32 {
	if !v.valid() {
		return 0
	}
	release := v.ctx.iso.mustEnter()
	defer release()
	return uint32(C.GV8ValueLen(v.ptr))
}

func (v *Value) Object() *Object {
	if v == nil || !v.IsObject() {
		return nil
	}
	return &Object{Value: v}
}

func (v *Value) Function() *Function {
	if v == nil || !v.IsFunction() {
		return nil
	}
	return &Function{Object: &Object{Value: v}}
}

func (v *Value) Promise() *Promise {
	if v == nil || !v.IsPromise() {
		return nil
	}
	return &Promise{Value: v}
}

// Object wraps a JavaScript object value.
type Object struct {
	*Value
}

func (o *Object) Get(name string) (*Value, error) {
	if o == nil || !o.valid() {
		return nil, invalidValueError()
	}
	release, err := o.ctx.iso.enter()
	if err != nil {
		return nil, err
	}
	defer release()
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))

	rtn := C.GV8ObjectGet(o.ptr, cname)
	return valueResult(o.ctx, rtn)
}

func (o *Object) GetIdx(index uint32) (*Value, error) {
	if o == nil || !o.valid() {
		return nil, invalidValueError()
	}
	release, err := o.ctx.iso.enter()
	if err != nil {
		return nil, err
	}
	defer release()
	rtn := C.GV8ObjectGetIdx(o.ptr, C.uint32_t(index))
	return valueResult(o.ctx, rtn)
}

// Has reports whether the object has a property named name.
func (o *Object) Has(name string) bool {
	if o == nil || !o.valid() {
		return false
	}
	release := o.ctx.iso.mustEnter()
	defer release()
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	return C.GV8ObjectHas(o.ptr, cname) != 0
}

// Len returns the JavaScript length for array-like objects.
func (o *Object) Len() uint32 {
	if o == nil || !o.valid() {
		return 0
	}
	return o.Value.Len()
}

func (o *Object) Set(name string, value any) error {
	if o == nil || !o.valid() {
		return invalidValueError()
	}
	release, err := o.ctx.iso.enter()
	if err != nil {
		return err
	}
	defer release()
	prop, cleanup, err := materializeValue(o.ctx, value)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return err
	}

	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	return newJSError(C.GV8ObjectSet(o.ptr, cname, prop.ptr))
}

// SetIdx sets an indexed property on the object.
func (o *Object) SetIdx(index uint32, value any) error {
	if o == nil || !o.valid() {
		return invalidValueError()
	}
	release, err := o.ctx.iso.enter()
	if err != nil {
		return err
	}
	defer release()
	prop, cleanup, err := materializeValue(o.ctx, value)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return err
	}
	return newJSError(C.GV8ObjectSetIdx(o.ptr, C.uint32_t(index), prop.ptr))
}

func (o *Object) CallMethod(name string, args ...any) (*Value, error) {
	if o == nil || !o.valid() {
		return nil, invalidValueError()
	}
	release, err := o.ctx.iso.enter()
	if err != nil {
		return nil, err
	}
	defer release()
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))

	argv, cleanup, err := materializeArgs(o.ctx, args)
	defer cleanup()
	if err != nil {
		return nil, err
	}
	cargv, freeArgv := cArgv(argv)
	defer freeArgv()

	rtn := C.GV8ObjectCallMethod(o.ptr, cname, C.int(len(argv)), cargv)
	return valueResult(o.ctx, rtn)
}

type Function struct {
	*Object
}

func (f *Function) Call(recv *Object, args ...any) (*Value, error) {
	if f == nil || !f.valid() {
		return nil, invalidValueError()
	}
	release, err := f.ctx.iso.enter()
	if err != nil {
		return nil, err
	}
	defer release()
	argv, cleanup, err := materializeArgs(f.ctx, args)
	defer cleanup()
	if err != nil {
		return nil, err
	}
	cargv, freeArgv := cArgv(argv)
	defer freeArgv()
	rtn := C.GV8ValueCall(f.ptr, recv.ptr, C.int(len(argv)), cargv)
	return valueResult(f.ctx, rtn)
}

func JSONStringify(ctx *Context, value *Value) (string, error) {
	if err := ctx.ensureOpen(); err != nil {
		return "", err
	}
	if value == nil || !value.valid() {
		return "", invalidValueError()
	}
	release, err := ctx.iso.enter()
	if err != nil {
		return "", err
	}
	defer release()
	rtn := C.GV8JSONStringify(ctx.ptr, value.ptr)
	if rtn.error.msg != nil {
		return "", newJSError(rtn.error)
	}
	defer C.free(unsafe.Pointer(rtn.data))
	return C.GoStringN(rtn.data, rtn.length), nil
}

func JSONParse(ctx *Context, value string) (*Value, error) {
	if err := ctx.ensureOpen(); err != nil {
		return nil, err
	}
	release, err := ctx.iso.enter()
	if err != nil {
		return nil, err
	}
	defer release()
	cvalue := C.CString(value)
	defer C.free(unsafe.Pointer(cvalue))
	rtn := C.GV8JSONParse(ctx.ptr, cvalue, C.int(len(value)))
	return valueResult(ctx, rtn)
}
