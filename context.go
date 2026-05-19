package gv8

// #include <stdlib.h>
// #include "gv8.h"
import "C"

import (
	"fmt"
	"sync"
	"unsafe"
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

	mu                      sync.RWMutex
	closed                  bool
	hostFnIDs               map[int]struct{}
	liveValues              map[*Value]struct{}
	moduleResolversByScript map[int]ModuleResolver
	moduleResolversByName   map[string]ModuleResolver
}

// NewContext creates a new V8 context owned by iso.
func NewContext(iso *Isolate) *Context {
	if iso == nil {
		panic("gv8: nil isolate")
	}

	ctxMu.Lock()
	ctxSeq++
	ref := ctxSeq
	ctxMu.Unlock()

	ctx := &Context{
		ref:                     ref,
		ptr:                     C.GV8NewContext(iso.ptr, C.int(ref)),
		iso:                     iso,
		hostFnIDs:               map[int]struct{}{},
		liveValues:              map[*Value]struct{}{},
		moduleResolversByScript: map[int]ModuleResolver{},
		moduleResolversByName:   map[string]ModuleResolver{},
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

// Close releases the native context and invalidates any live values created
// from it.
func (c *Context) Close() {
	if c == nil || c.ptr == nil {
		return
	}

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	hostFnIDs := make([]int, 0, len(c.hostFnIDs))
	for id := range c.hostFnIDs {
		hostFnIDs = append(hostFnIDs, id)
	}
	liveValues := make([]*Value, 0, len(c.liveValues))
	for value := range c.liveValues {
		liveValues = append(liveValues, value)
	}
	c.hostFnIDs = nil
	c.liveValues = nil
	c.moduleResolversByScript = nil
	c.moduleResolversByName = nil
	c.mu.Unlock()

	for _, value := range liveValues {
		value.invalidate()
	}
	unregisterHostFunctions(hostFnIDs)

	deleteContext(c.ref)
	C.GV8ContextDispose(c.ptr)
	c.ptr = nil
}

func (c *Context) bindModuleResolver(module *Module, resolver ModuleResolver) {
	if c == nil || module == nil || resolver == nil {
		return
	}

	scriptID := module.ScriptID()
	resourceName := module.ResourceName()

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	if scriptID != 0 && c.moduleResolversByScript != nil {
		c.moduleResolversByScript[scriptID] = resolver
	}
	if resourceName != "" && c.moduleResolversByName != nil {
		c.moduleResolversByName[resourceName] = resolver
	}
}

func (c *Context) moduleResolverForScript(scriptID int) ModuleResolver {
	if c == nil || scriptID == 0 {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.moduleResolversByScript == nil {
		return nil
	}
	return c.moduleResolversByScript[scriptID]
}

func (c *Context) moduleResolverForResource(resourceName string) ModuleResolver {
	if c == nil || resourceName == "" {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.moduleResolversByName == nil {
		return nil
	}
	return c.moduleResolversByName[resourceName]
}

func (c *Context) trackHostFunction(id int) {
	if c == nil || id == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.hostFnIDs == nil {
		unregisterHostFunctions([]int{id})
		return
	}
	c.hostFnIDs[id] = struct{}{}
}

func (c *Context) trackValue(value *Value) {
	if c == nil || value == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.liveValues == nil {
		value.invalidate()
		return
	}
	c.liveValues[value] = struct{}{}
}

func (c *Context) untrackValue(value *Value) {
	if c == nil || value == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.liveValues == nil {
		return
	}
	delete(c.liveValues, value)
}

func (c *Context) isClosed() bool {
	if c == nil {
		return true
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.closed || c.ptr == nil
}

func (c *Context) ensureOpen() error {
	if c == nil {
		return fmt.Errorf("gv8: nil context")
	}
	if c.isClosed() {
		return fmt.Errorf("gv8: context is closed")
	}
	return nil
}

func (c *Context) Isolate() *Isolate {
	return c.iso
}

// Global returns the global object for the context.
func (c *Context) Global() *Object {
	if c == nil || c.isClosed() {
		return nil
	}
	release := c.iso.mustEnter()
	defer release()
	return &Object{Value: newValue(c, C.GV8ContextGlobal(c.ptr))}
}

// Deprecated: prefer Global().Get(name) so global-object ownership remains
// explicit at the callsite.
func (c *Context) GetGlobal(name string) (*Value, error) {
	if err := c.ensureOpen(); err != nil {
		return nil, err
	}
	global := c.Global()
	defer global.Release()
	return global.Get(name)
}

// Deprecated: prefer Global().Set(name, value) so global-object ownership
// remains explicit at the callsite.
func (c *Context) SetGlobal(name string, value any) error {
	if err := c.ensureOpen(); err != nil {
		return err
	}
	global := c.Global()
	defer global.Release()
	return global.Set(name, value)
}

// Deprecated: prefer repeated explicit Global().Set calls so ownership and
// failure points stay visible.
func (c *Context) SetGlobals(values map[string]any) error {
	if err := c.ensureOpen(); err != nil {
		return err
	}
	for name, value := range values {
		if err := c.SetGlobal(name, value); err != nil {
			return err
		}
	}
	return nil
}

// Deprecated: prefer NewFunction plus Global().Set so function lifetime stays
// explicit.
func (c *Context) Bind(name string, fn HostFunction) error {
	hostFn, err := NewFunction(c, fn)
	if err != nil {
		return err
	}
	defer hostFn.Release()
	return c.SetGlobal(name, hostFn)
}

// NewObject allocates a new object in the context.
func (c *Context) NewObject() (*Object, error) {
	if err := c.ensureOpen(); err != nil {
		return nil, err
	}
	release, err := c.iso.enter()
	if err != nil {
		return nil, err
	}
	defer release()
	rtn := C.GV8ContextNewObject(c.ptr)
	value, err := valueResult(c, rtn)
	if err != nil {
		return nil, err
	}
	return value.Object(), nil
}

// RunScript compiles and executes source in the context.
func (c *Context) RunScript(source string, origin string) (*Value, error) {
	if err := c.ensureOpen(); err != nil {
		return nil, err
	}
	release, err := c.iso.enter()
	if err != nil {
		return nil, err
	}
	defer release()

	csource := C.CString(source)
	corigin := C.CString(origin)
	defer C.free(unsafe.Pointer(csource))
	defer C.free(unsafe.Pointer(corigin))

	rtn := C.GV8ContextRunScript(c.ptr, csource, corigin, 0, 0)
	return valueResult(c, rtn)
}
