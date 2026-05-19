package gv8

// #include <stdlib.h>
// #include "gv8.h"
import "C"

import (
	"fmt"
	"sync"
	"unsafe"
)

type HostFunction func(*FunctionCallbackInfo) (*Value, error)

type FunctionCallbackInfo struct {
	ctx  *Context
	this *Object
	args []*Value
}

func (i *FunctionCallbackInfo) Context() *Context {
	return i.ctx
}

func (i *FunctionCallbackInfo) This() *Object {
	return i.this
}

func (i *FunctionCallbackInfo) Args() []*Value {
	return i.args
}

var (
	hostFnMu   sync.RWMutex
	hostFnSeq  int
	hostFnRefs = map[int]HostFunction{}
)

func registerHostFunction(fn HostFunction) int {
	hostFnMu.Lock()
	defer hostFnMu.Unlock()
	hostFnSeq++
	hostFnRefs[hostFnSeq] = fn
	return hostFnSeq
}

func unregisterHostFunctions(ids []int) {
	if len(ids) == 0 {
		return
	}
	hostFnMu.Lock()
	defer hostFnMu.Unlock()
	for _, id := range ids {
		delete(hostFnRefs, id)
	}
}

func lookupHostFunction(id int) HostFunction {
	hostFnMu.RLock()
	defer hostFnMu.RUnlock()
	return hostFnRefs[id]
}

func NewFunction(ctx *Context, fn HostFunction) (*Function, error) {
	if err := ctx.ensureOpen(); err != nil {
		return nil, err
	}
	if fn == nil {
		return nil, fmt.Errorf("gv8: nil host function")
	}

	callbackID := registerHostFunction(fn)
	ctx.trackHostFunction(callbackID)
	release, err := ctx.iso.enter()
	if err != nil {
		unregisterHostFunctions([]int{callbackID})
		return nil, err
	}
	defer release()
	rtn := C.GV8ContextNewFunction(ctx.ptr, C.int(callbackID))
	value, err := valueResult(ctx, rtn)
	if err != nil {
		unregisterHostFunctions([]int{callbackID})
		return nil, err
	}
	return value.Function(), nil
}

func materializeValue(ctx *Context, value any) (*Value, func(), error) {
	switch v := value.(type) {
	case nil:
		next, err := NewNullValue(ctx)
		return next, func() { next.Release() }, err
	case *Value:
		return v, nil, nil
	case *Object:
		return v.Value, nil, nil
	case *Function:
		return v.Value, nil, nil
	case *Promise:
		return v.Value, nil, nil
	case string:
		next, err := NewStringValue(ctx, v)
		return next, func() { next.Release() }, err
	case bool:
		next, err := NewBooleanValue(ctx, v)
		return next, func() { next.Release() }, err
	case int:
		next, err := NewIntegerValue(ctx, int64(v))
		return next, func() { next.Release() }, err
	case int64:
		next, err := NewIntegerValue(ctx, v)
		return next, func() { next.Release() }, err
	case uint32:
		next, err := NewIntegerValue(ctx, int64(v))
		return next, func() { next.Release() }, err
	default:
		return nil, nil, fmt.Errorf("gv8: unsupported value type %T", value)
	}
}

func materializeArgs(ctx *Context, values []any) ([]*Value, func(), error) {
	args := make([]*Value, 0, len(values))
	cleanups := make([]func(), 0, len(values))
	for _, value := range values {
		next, cleanup, err := materializeValue(ctx, value)
		if err != nil {
			for _, fn := range cleanups {
				fn()
			}
			return nil, func() {}, err
		}
		args = append(args, next)
		if cleanup != nil {
			cleanups = append(cleanups, cleanup)
		}
	}
	return args, func() {
		for _, fn := range cleanups {
			fn()
		}
	}, nil
}

func cArgv(args []*Value) (*C.GV8ValuePtr, func()) {
	if len(args) == 0 {
		return nil, func() {}
	}
	mem := C.malloc(C.size_t(len(args)) * C.size_t(unsafe.Sizeof(C.GV8ValuePtr(nil))))
	slice := unsafe.Slice((*C.GV8ValuePtr)(mem), len(args))
	for i, value := range args {
		slice[i] = value.ptr
	}
	return (*C.GV8ValuePtr)(mem), func() { C.free(mem) }
}

//export gv8FunctionCallback
func gv8FunctionCallback(ctxRef C.int, callbackID C.int, recv C.GV8ValuePtr, argc C.int, argv *C.GV8ValuePtr) C.GV8CallbackResult {
	ctx := getContext(int(ctxRef))
	fn := lookupHostFunction(int(callbackID))
	if ctx == nil || fn == nil {
		errValue, _ := newErrorValue(ctx, "gv8: host function callback not found")
		if errValue != nil {
			return C.GV8CallbackResult{error_value: errValue.ptr}
		}
		return C.GV8CallbackResult{}
	}

	args := make([]*Value, int(argc))
	if argc > 0 && argv != nil {
		argSlice := unsafe.Slice(argv, int(argc))
		for i, ptr := range argSlice {
			args[i] = newValue(ctx, ptr)
		}
	}

	result, err := fn(&FunctionCallbackInfo{
		ctx:  ctx,
		this: newValue(ctx, recv).Object(),
		args: args,
	})
	if err != nil {
		errValue, valueErr := newErrorValue(ctx, err.Error())
		if valueErr != nil || errValue == nil {
			return C.GV8CallbackResult{}
		}
		return C.GV8CallbackResult{error_value: errValue.ptr}
	}
	if result == nil {
		return C.GV8CallbackResult{}
	}
	return C.GV8CallbackResult{value: result.ptr}
}
