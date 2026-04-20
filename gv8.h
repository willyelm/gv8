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

extern void GV8Init();
extern GV8IsolatePtr GV8NewIsolate();
extern void GV8IsolateDispose(GV8IsolatePtr iso);
extern void GV8IsolatePerformMicrotaskCheckpoint(GV8IsolatePtr iso);

extern GV8ContextPtr GV8NewContext(GV8IsolatePtr iso, int ref);
extern void GV8ContextDispose(GV8ContextPtr ctx);
extern GV8ValuePtr GV8ContextGlobal(GV8ContextPtr ctx);

extern GV8RtnUnboundScript GV8CompileUnboundScript(GV8IsolatePtr iso,
                                                   const char* source,
                                                   const char* resource_name,
                                                   int line_offset,
                                                   int column_offset);
extern GV8RtnValue GV8UnboundScriptRun(GV8ContextPtr ctx,
                                       GV8UnboundScriptPtr script);

extern GV8RtnModule GV8CompileModule(GV8IsolatePtr iso,
                                     const char* source,
                                     const char* resource_name,
                                     int line_offset,
                                     int column_offset);
extern GV8RtnError GV8ModuleInstantiate(GV8ContextPtr ctx, GV8ModulePtr module);
extern GV8RtnValue GV8ModuleEvaluate(GV8ContextPtr ctx, GV8ModulePtr module);
extern GV8ValuePtr GV8ModuleGetNamespace(GV8ContextPtr ctx, GV8ModulePtr module);

extern GV8RtnValue GV8NewString(GV8ContextPtr ctx, const char* value, int length);
extern void GV8ValueRelease(GV8ValuePtr value);
extern GV8RtnString GV8ValueToString(GV8ValuePtr value);
extern int64_t GV8ValueToInteger(GV8ValuePtr value);
extern GV8RtnValue GV8ObjectGet(GV8ValuePtr value, const char* key);

extern GV8ResolvedModule gv8ResolveModuleCallback(int ctx_ref,
                                                  char* specifier,
                                                  GV8ModulePtr referrer);

#ifdef __cplusplus
}
#endif

#endif
