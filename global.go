package gv8

// #include "gv8.h"
import "C"

import "fmt"

// GlobalValue is a persistent V8 handle for a value in its original context.
type GlobalValue struct {
	value *Value
}

// NewGlobalValue keeps value alive until the returned handle is released.
func NewGlobalValue(value *Value) (*GlobalValue, error) {
	if value == nil || !value.valid() {
		return nil, invalidValueError()
	}
	release, err := value.ctx.iso.enter()
	if err != nil {
		return nil, err
	}
	defer release()
	ptr := C.GV8ValueRetain(value.ptr)
	if ptr == nil {
		return nil, invalidValueError()
	}
	return &GlobalValue{value: newValue(value.ctx, ptr)}, nil
}

// Value creates a local handle for the persistent value in ctx.
func (g *GlobalValue) Value(ctx *Context) (*Value, error) {
	if g == nil || g.value == nil || !g.value.valid() {
		return nil, invalidValueError()
	}
	if ctx == nil || ctx != g.value.ctx {
		return nil, fmt.Errorf("gv8: global value belongs to a different context")
	}
	if err := ctx.ensureOpen(); err != nil {
		return nil, err
	}
	release, err := ctx.iso.enter()
	if err != nil {
		return nil, err
	}
	defer release()
	ptr := C.GV8ValueRetain(g.value.ptr)
	if ptr == nil {
		return nil, invalidValueError()
	}
	return newValue(ctx, ptr), nil
}

// Release drops the persistent V8 handle.
func (g *GlobalValue) Release() {
	if g == nil || g.value == nil {
		return
	}
	g.value.Release()
	g.value = nil
}

// GlobalFunction is a persistent handle for a JavaScript function.
type GlobalFunction struct {
	*GlobalValue
}

// NewGlobalFunction keeps fn alive until the returned handle is released.
func NewGlobalFunction(fn *Function) (*GlobalFunction, error) {
	if fn == nil || !fn.valid() {
		return nil, invalidValueError()
	}
	value, err := NewGlobalValue(fn.Value)
	if err != nil {
		return nil, err
	}
	return &GlobalFunction{GlobalValue: value}, nil
}

// Function creates a local function handle in ctx.
func (g *GlobalFunction) Function(ctx *Context) (*Function, error) {
	value, err := g.Value(ctx)
	if err != nil {
		return nil, err
	}
	fn := value.Function()
	if fn == nil {
		value.Release()
		return nil, fmt.Errorf("gv8: global value is not a function")
	}
	return fn, nil
}
