// Copyright 2026 The gv8 authors.

#ifndef GV8_H
#define GV8_H

#ifdef __cplusplus
#include "libplatform/libplatform.h"
#include "v8.h"

typedef v8::Isolate* GV8IsolatePtr;
extern "C" {
#else
typedef struct gv8Isolate gv8Isolate;
typedef gv8Isolate* GV8IsolatePtr;
#endif

#include <stdint.h>

typedef struct gv8_ctx* GV8ContextPtr;
typedef struct gv8_value* GV8ValuePtr;
typedef struct gv8_unbound_script* GV8UnboundScriptPtr;
typedef struct gv8_module* GV8ModulePtr;
typedef struct gv8_promise_resolver* GV8PromiseResolverPtr;

typedef struct {
  const char* msg;
  const char* location;
  const char* stack;
} GV8RtnError;

typedef struct {
  GV8ValuePtr value;
  GV8RtnError error;
} GV8RtnValue;

typedef struct {
  GV8ModulePtr module;
  GV8ValuePtr error_value;
} GV8ResolvedModule;

typedef struct {
  GV8ValuePtr value;
  GV8ValuePtr error_value;
} GV8CallbackResult;

typedef struct {
  GV8ValuePtr promise;
  GV8ValuePtr error_value;
} GV8DynamicImportResult;

typedef struct {
  GV8UnboundScriptPtr ptr;
  GV8RtnError error;
} GV8RtnUnboundScript;

typedef struct {
  GV8ModulePtr ptr;
  GV8RtnError error;
} GV8RtnModule;

typedef struct {
  const char* data;
  int length;
  GV8RtnError error;
} GV8RtnString;

typedef struct {
  uint8_t* data;
  int length;
  GV8RtnError error;
} GV8RtnBytes;

extern void GV8Init();
extern GV8IsolatePtr GV8NewIsolate();
extern void GV8IsolateDispose(GV8IsolatePtr iso);
extern void GV8IsolatePerformMicrotaskCheckpoint(GV8IsolatePtr iso);

extern GV8ContextPtr GV8NewContext(GV8IsolatePtr iso, int ref);
extern void GV8ContextDispose(GV8ContextPtr ctx);
extern GV8ValuePtr GV8ContextGlobal(GV8ContextPtr ctx);
extern GV8RtnValue GV8ContextNewObject(GV8ContextPtr ctx);
extern GV8RtnValue GV8ContextNewFunction(GV8ContextPtr ctx, int callback_id);
extern GV8RtnValue GV8ContextNewPromiseResolver(GV8ContextPtr ctx);
extern GV8RtnValue GV8ContextRunScript(GV8ContextPtr ctx,
                                       const char* source,
                                       const char* resource_name,
                                       int line_offset,
                                       int column_offset);

extern GV8RtnUnboundScript GV8CompileUnboundScript(GV8IsolatePtr iso,
                                                   const char* source,
                                                   const char* resource_name,
                                                   int line_offset,
                                                   int column_offset);
extern void GV8UnboundScriptRelease(GV8UnboundScriptPtr script);
extern GV8RtnValue GV8UnboundScriptRun(GV8ContextPtr ctx,
                                       GV8UnboundScriptPtr script);

extern GV8RtnModule GV8CompileModule(GV8IsolatePtr iso,
                                     const char* source,
                                     const char* resource_name,
                                     int line_offset,
                                     int column_offset);
extern void GV8ModuleRelease(GV8ModulePtr module);
extern GV8RtnError GV8ModuleInstantiate(GV8ContextPtr ctx, GV8ModulePtr module);
extern GV8RtnValue GV8ModuleEvaluate(GV8ContextPtr ctx, GV8ModulePtr module);
extern GV8ValuePtr GV8ModuleGetNamespace(GV8ContextPtr ctx, GV8ModulePtr module);
extern int GV8ModuleGetStatus(GV8ModulePtr module);
extern int GV8ModuleGetScriptID(GV8ModulePtr module);

extern GV8RtnValue GV8NewString(GV8ContextPtr ctx, const char* value, int length);
extern GV8RtnValue GV8NewBoolean(GV8ContextPtr ctx, int value);
extern GV8RtnValue GV8NewInteger(GV8ContextPtr ctx, int64_t value);
extern GV8RtnValue GV8NewNull(GV8ContextPtr ctx);
extern GV8RtnValue GV8NewUndefined(GV8ContextPtr ctx);
extern GV8RtnValue GV8NewError(GV8ContextPtr ctx, const char* value, int length);
extern void GV8ValueRelease(GV8ValuePtr value);
extern GV8RtnString GV8ValueToString(GV8ValuePtr value);
extern int64_t GV8ValueToInteger(GV8ValuePtr value);
extern int GV8ValueToBoolean(GV8ValuePtr value);
extern int GV8ValueIsObject(GV8ValuePtr value);
extern int GV8ValueIsString(GV8ValuePtr value);
extern int GV8ValueIsBoolean(GV8ValuePtr value);
extern int GV8ValueIsUndefined(GV8ValuePtr value);
extern int GV8ValueIsNull(GV8ValuePtr value);
extern int GV8ValueIsPromise(GV8ValuePtr value);
extern int GV8ValueIsFunction(GV8ValuePtr value);
extern int GV8ValueIsArrayBuffer(GV8ValuePtr value);
extern int GV8ValueIsArrayBufferView(GV8ValuePtr value);
extern int GV8ValueIsUint8Array(GV8ValuePtr value);
extern int GV8ValueIsInt8Array(GV8ValuePtr value);
extern int GV8ValueIsUint8ClampedArray(GV8ValuePtr value);
extern GV8RtnBytes GV8ValueBytes(GV8ValuePtr value);
extern GV8RtnValue GV8ObjectGet(GV8ValuePtr value, const char* key);
extern GV8RtnValue GV8ObjectGetIdx(GV8ValuePtr value, uint32_t index);
extern GV8RtnError GV8ObjectSet(GV8ValuePtr value, const char* key, GV8ValuePtr prop);
extern GV8RtnValue GV8ValueCall(GV8ValuePtr value,
                                GV8ValuePtr recv,
                                int argc,
                                GV8ValuePtr* argv);
extern GV8RtnValue GV8ObjectCallMethod(GV8ValuePtr value,
                                       const char* key,
                                       int argc,
                                       GV8ValuePtr* argv);
extern int GV8PromiseState(GV8ValuePtr value);
extern GV8ValuePtr GV8PromiseResult(GV8ValuePtr value);
extern GV8ValuePtr GV8PromiseResolverGetPromise(GV8ValuePtr value);
extern GV8RtnError GV8PromiseResolverResolve(GV8ValuePtr value, GV8ValuePtr result);
extern GV8RtnError GV8PromiseResolverReject(GV8ValuePtr value, GV8ValuePtr result);
extern GV8RtnString GV8JSONStringify(GV8ContextPtr ctx, GV8ValuePtr value);
extern GV8RtnValue GV8JSONParse(GV8ContextPtr ctx, const char* value, int length);

extern GV8ResolvedModule gv8ResolveModuleCallback(int ctx_ref,
                                                  char* specifier,
                                                  GV8ModulePtr referrer);
extern GV8DynamicImportResult gv8ResolveDynamicImportCallback(
    int ctx_ref,
    char* specifier,
    char* resource_name);
extern GV8CallbackResult gv8FunctionCallback(int ctx_ref,
                                             int callback_id,
                                             GV8ValuePtr recv,
                                             int argc,
                                             GV8ValuePtr* argv);

#ifdef __cplusplus
}
#endif

#endif
