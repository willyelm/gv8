package gv8

// #include <stdlib.h>
// #include "gv8.h"
import "C"

import (
	"context"
	"unsafe"
)

// Module is a compiled ECMAScript module bound to an isolate.
type Module struct {
	ptr          C.GV8ModulePtr
	iso          *Isolate
	resourceName string
}

type ModuleResolver interface {
	ResolveModule(ctx *Context, specifier string, referrer *Module) (*Module, error)
}

type DynamicImportResolver interface {
	ResolveDynamicImport(ctx *Context, specifier string, resourceName string) (*Promise, error)
}

type ModuleStatus int

const (
	ModuleStatusUninstantiated ModuleStatus = iota
	ModuleStatusInstantiating
	ModuleStatusInstantiated
	ModuleStatusEvaluating
	ModuleStatusEvaluated
	ModuleStatusErrored
)

func compileModule(iso *Isolate, source string, origin ScriptOrigin) (*Module, error) {
	release, err := iso.enter()
	if err != nil {
		return nil, err
	}
	defer release()

	csource := C.CString(source)
	corigin := C.CString(origin.ResourceName)
	defer C.free(unsafe.Pointer(csource))
	defer C.free(unsafe.Pointer(corigin))

	rtn := C.GV8CompileModule(
		iso.ptr,
		csource,
		corigin,
		C.int(origin.LineOffset),
		C.int(origin.ColumnOffset),
	)
	if rtn.ptr == nil {
		return nil, observeJSError(iso, newJSError(rtn.error))
	}
	return &Module{ptr: rtn.ptr, iso: iso, resourceName: origin.ResourceName}, nil
}

func (m *Module) Release() {
	if m == nil || m.ptr == nil {
		return
	}
	release := m.iso.mustEnter()
	defer release()
	C.GV8ModuleRelease(m.ptr)
	m.ptr = nil
}

// Instantiate links the module using resolver.
func (m *Module) Instantiate(ctx *Context, resolver ModuleResolver) error {
	if m == nil || m.ptr == nil {
		return invalidModuleError()
	}
	if err := ctx.ensureOpen(); err != nil {
		return err
	}
	release, err := ctx.iso.enter()
	if err != nil {
		return err
	}
	defer release()
	ctx.bindModuleResolver(m, resolver)
	rtn := C.GV8ModuleInstantiate(ctx.ptr, m.ptr)
	if rtn.msg == nil {
		return nil
	}
	return observeJSError(ctx.iso, newJSError(rtn))
}

// Evaluate evaluates the already instantiated module.
func (m *Module) Evaluate(ctx *Context) (*Value, error) {
	if m == nil || m.ptr == nil {
		return nil, invalidModuleError()
	}
	if err := ctx.ensureOpen(); err != nil {
		return nil, err
	}
	release, err := ctx.iso.enter()
	if err != nil {
		return nil, err
	}
	defer release()
	rtn := C.GV8ModuleEvaluate(ctx.ptr, m.ptr)
	return valueResult(ctx, rtn)
}

func (m *Module) EnsureInstantiated(ctx *Context, resolver ModuleResolver) error {
	if m == nil {
		return &JSError{Message: "gv8: nil module"}
	}
	if m.Status() != ModuleStatusUninstantiated {
		return nil
	}
	return m.Instantiate(ctx, resolver)
}

// Deprecated: prefer explicit Instantiate followed by Evaluate so lifecycle
// stages remain visible.
func (m *Module) InstantiateAndEvaluate(ctx *Context, resolver ModuleResolver, pump func(context.Context) error) (*Value, error) {
	if err := m.EnsureInstantiated(ctx, resolver); err != nil {
		return nil, err
	}

	value, err := m.Evaluate(ctx)
	if err != nil {
		return nil, err
	}
	if value == nil || !value.IsPromise() {
		return value, nil
	}
	return value.Promise().Await(context.Background(), pump)
}

// Deprecated: prefer explicit Evaluate followed by Namespace so lifecycle
// stages remain visible.
func (m *Module) EvaluateNamespace(ctx *Context, pump func(context.Context) error) (*Object, error) {
	value, err := m.Evaluate(ctx)
	if err != nil {
		return nil, err
	}
	if value != nil && value.IsPromise() {
		if _, err := value.Promise().Await(context.Background(), pump); err != nil {
			return nil, err
		}
	}
	return m.Namespace(ctx), nil
}

// Deprecated: prefer explicit Instantiate, Evaluate, and Namespace calls so
// lifecycle stages remain visible.
func (m *Module) ReadyNamespace(ctx *Context, resolver ModuleResolver, pump func(context.Context) error) (*Object, error) {
	if err := m.EnsureInstantiated(ctx, resolver); err != nil {
		return nil, err
	}

	switch m.Status() {
	case ModuleStatusEvaluated:
		return m.Namespace(ctx), nil
	case ModuleStatusInstantiated:
		return m.EvaluateNamespace(ctx, pump)
	case ModuleStatusInstantiating, ModuleStatusEvaluating:
		return nil, &JSError{Message: "gv8: module is still being initialized"}
	case ModuleStatusErrored:
		return nil, &JSError{Message: "gv8: module is in an errored state"}
	default:
		return nil, &JSError{Message: "gv8: unsupported module status"}
	}
}

func (m *Module) Namespace(ctx *Context) *Object {
	if m == nil || m.ptr == nil || ctx == nil || ctx.isClosed() {
		return nil
	}
	release := ctx.iso.mustEnter()
	defer release()
	return &Object{Value: newValue(ctx, C.GV8ModuleGetNamespace(ctx.ptr, m.ptr))}
}

func (m *Module) Status() ModuleStatus {
	if m == nil || m.ptr == nil {
		return ModuleStatusUninstantiated
	}
	release := m.iso.mustEnter()
	defer release()
	return ModuleStatus(C.GV8ModuleGetStatus(m.ptr))
}

func (m *Module) ScriptID() int {
	if m == nil || m.ptr == nil {
		return 0
	}
	release := m.iso.mustEnter()
	defer release()
	return int(C.GV8ModuleGetScriptID(m.ptr))
}

func (m *Module) ResourceName() string {
	if m == nil {
		return ""
	}
	return m.resourceName
}

//export gv8ResolveModuleCallback
func gv8ResolveModuleCallback(ctxRef C.int, spec *C.char, referrer C.GV8ModulePtr) C.GV8ResolvedModule {
	ctx := getContext(int(ctxRef))
	if ctx == nil {
		return C.GV8ResolvedModule{}
	}
	referrerModule := &Module{
		ptr: referrer,
		iso: ctx.iso,
	}
	resolver := ctx.moduleResolverForScript(referrerModule.ScriptID())
	if resolver == nil {
		ctx.iso.noteModuleResolutionFailure(C.GoString(spec), referrerModule.ResourceName(), &JSError{Message: "gv8: no module resolver configured"})
		msg, _ := newErrorValue(ctx, "gv8: no module resolver configured")
		if msg != nil {
			return C.GV8ResolvedModule{error_value: msg.ptr}
		}
		return C.GV8ResolvedModule{}
	}

	mod, err := resolver.ResolveModule(ctx, C.GoString(spec), referrerModule)
	if err == nil {
		ctx.bindModuleResolver(mod, resolver)
		return C.GV8ResolvedModule{module: mod.ptr}
	}
	ctx.iso.noteModuleResolutionFailure(C.GoString(spec), referrerModule.ResourceName(), err)

	msg, msgErr := newErrorValue(ctx, "cannot resolve module: "+err.Error())
	if msgErr != nil {
		return C.GV8ResolvedModule{}
	}
	return C.GV8ResolvedModule{error_value: msg.ptr}
}

//export gv8ResolveDynamicImportCallback
func gv8ResolveDynamicImportCallback(ctxRef C.int, spec *C.char, resourceName *C.char) C.GV8DynamicImportResult {
	ctx := getContext(int(ctxRef))
	if ctx == nil {
		return C.GV8DynamicImportResult{}
	}
	resolver := ctx.moduleResolverForResource(C.GoString(resourceName))
	if resolver == nil {
		ctx.iso.noteModuleResolutionFailure(C.GoString(spec), C.GoString(resourceName), &JSError{Message: "gv8: no dynamic import resolver configured"})
		msg, _ := newErrorValue(ctx, "gv8: no dynamic import resolver configured")
		if msg != nil {
			return C.GV8DynamicImportResult{error_value: msg.ptr}
		}
		return C.GV8DynamicImportResult{}
	}

	dynamicResolver, ok := resolver.(DynamicImportResolver)
	if !ok {
		ctx.iso.noteModuleResolutionFailure(C.GoString(spec), C.GoString(resourceName), &JSError{Message: "gv8: module resolver does not support dynamic imports"})
		msg, _ := newErrorValue(ctx, "gv8: module resolver does not support dynamic imports")
		if msg != nil {
			return C.GV8DynamicImportResult{error_value: msg.ptr}
		}
		return C.GV8DynamicImportResult{}
	}

	promise, err := dynamicResolver.ResolveDynamicImport(ctx, C.GoString(spec), C.GoString(resourceName))
	if err == nil && promise != nil {
		return C.GV8DynamicImportResult{promise: promise.ptr}
	}
	if err == nil {
		err = &JSError{Message: "dynamic import resolver returned nil promise"}
	}
	ctx.iso.noteModuleResolutionFailure(C.GoString(spec), C.GoString(resourceName), err)

	msg, msgErr := newErrorValue(ctx, "cannot resolve dynamic import: "+err.Error())
	if msgErr != nil {
		return C.GV8DynamicImportResult{}
	}
	return C.GV8DynamicImportResult{error_value: msg.ptr}
}
