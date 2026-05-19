// Copyright 2026 The gv8 authors.

#include "gv8.h"

#include <cstdlib>
#include <cstring>
#include <dlfcn.h>
#include <sstream>
#include <string>
#include <unordered_map>
#include <vector>

using namespace v8;

auto default_platform = platform::NewDefaultPlatform();
ArrayBuffer::Allocator* default_allocator;
constexpr ExternalPointerTypeTag kContextWrapTag =
    kExternalPointerTypeTagDefault;

struct gv8_ctx {
  Isolate* iso;
  Persistent<Context> ptr;
  int ref;
  std::unordered_map<long, gv8_value*> values;
  long next_value_id;
};

struct gv8_value {
  long id;
  gv8_ctx* ctx;
  Isolate* iso;
  Persistent<Value> ptr;
};

struct gv8_unbound_script {
  Isolate* iso;
  Persistent<UnboundScript> ptr;
};

struct gv8_module {
  Isolate* iso;
  Persistent<Module> ptr;
};

Local<Value> NewError(Isolate* iso, Local<Context> ctx, const std::string& msg) {
  Local<String> text =
      String::NewFromUtf8(iso, msg.c_str(), NewStringType::kNormal)
          .ToLocalChecked();
  return Exception::Error(text);
}

std::string SharedLibraryDir() {
  Dl_info info;
  if (dladdr(reinterpret_cast<void*>(&GV8Init), &info) == 0 ||
      info.dli_fname == nullptr) {
    return ".";
  }

  std::string path(info.dli_fname);
  std::string::size_type slash = path.rfind('/');
  if (slash == std::string::npos) {
    return ".";
  }
  return path.substr(0, slash);
}

const char* CopyString(const std::string& str) {
  char* mem = static_cast<char*>(malloc(str.size() + 1));
  memcpy(mem, str.data(), str.size());
  mem[str.size()] = 0;
  return mem;
}

const char* CopyString(String::Utf8Value& value) {
  if (value.length() == 0) {
    return nullptr;
  }
  return CopyString(std::string(*value, value.length()));
}

static inline gv8_ctx* internalContext(Isolate* iso) {
  return static_cast<gv8_ctx*>(iso->GetData(0));
}

gv8_value* trackValue(gv8_ctx* ctx, gv8_value* value) {
  if (value->id == 0) {
    value->id = ++ctx->next_value_id;
    ctx->values[value->id] = value;
  }
  return value;
}

gv8_value* wrapValue(gv8_ctx* ctx, Isolate* iso, Local<Value> value) {
  gv8_value* wrap = new gv8_value;
  wrap->id = 0;
  wrap->ctx = ctx;
  wrap->iso = iso;
  wrap->ptr.Reset(iso, value);
  return trackValue(ctx, wrap);
}

GV8RtnError EmptyError() {
  GV8RtnError rtn = {nullptr, nullptr, nullptr};
  return rtn;
}

GV8RtnValue EmptyValueResult() {
  GV8RtnValue rtn = {};
  return rtn;
}

GV8RtnError ExceptionError(TryCatch& try_catch,
                           Isolate* iso,
                           Local<Context> ctx) {
  GV8RtnError rtn = {nullptr, nullptr, nullptr};

  String::Utf8Value exception(iso, try_catch.Exception());
  rtn.msg = CopyString(exception);

  Local<Message> msg = try_catch.Message();
  if (!msg.IsEmpty()) {
    std::ostringstream sb;
    String::Utf8Value origin(iso, msg->GetScriptOrigin().ResourceName());
    sb << *origin;

    Maybe<int> line = msg->GetLineNumber(ctx);
    if (line.IsJust()) {
      sb << ":" << line.FromJust();
    }

    Maybe<int> col = msg->GetStartColumn(ctx);
    if (col.IsJust()) {
      sb << ":" << (col.FromJust() + 1);
    }
    rtn.location = CopyString(sb.str());
  }

  Local<Value> stack;
  if (try_catch.StackTrace(ctx).ToLocal(&stack)) {
    String::Utf8Value utf8(iso, stack);
    rtn.stack = CopyString(utf8);
  }

  return rtn;
}

GV8RtnError ErrorFromMessage(Isolate* iso,
                             Local<Context> ctx,
                             Local<Message> msg,
                             Local<Value> value) {
  GV8RtnError rtn = {nullptr, nullptr, nullptr};

  if (!value.IsEmpty()) {
    String::Utf8Value exception(iso, value);
    rtn.msg = CopyString(exception);

    if (value->IsObject()) {
      Local<Value> stack;
      Local<String> key =
          String::NewFromUtf8(iso, "stack", NewStringType::kNormal)
              .ToLocalChecked();
      if (value.As<Object>()->Get(ctx, key).ToLocal(&stack)) {
        String::Utf8Value utf8(iso, stack);
        rtn.stack = CopyString(utf8);
      }
    }
  }

  if (!msg.IsEmpty()) {
    std::ostringstream sb;
    String::Utf8Value origin(iso, msg->GetScriptOrigin().ResourceName());
    sb << *origin;

    Maybe<int> line = msg->GetLineNumber(ctx);
    if (line.IsJust()) {
      sb << ":" << line.FromJust();
    }

    Maybe<int> col = msg->GetStartColumn(ctx);
    if (col.IsJust()) {
      sb << ":" << (col.FromJust() + 1);
    }
    rtn.location = CopyString(sb.str());

    if (rtn.msg == nullptr) {
      String::Utf8Value text(iso, msg->Get());
      rtn.msg = CopyString(text);
    }
  }

  return rtn;
}

ScriptOrigin MakeOrigin(Isolate* iso,
                        const char* resource_name,
                        int line_offset,
                        int column_offset,
                        bool is_module) {
  Local<String> name =
      String::NewFromUtf8(iso, resource_name, NewStringType::kNormal)
          .ToLocalChecked();
  return ScriptOrigin(name, line_offset, column_offset, false, -1,
                      Local<Value>(), false, false, is_module);
}

#define ISO_SCOPE(iso)              \
  Locker locker(iso);               \
  Isolate::Scope isolate_scope(iso); \
  HandleScope handle_scope(iso);

#define CTX_SCOPE(ctx_ptr)                   \
  Isolate* iso = ctx_ptr->iso;               \
  ISO_SCOPE(iso)                             \
  TryCatch try_catch(iso);                   \
  Local<Context> local_ctx = ctx_ptr->ptr.Get(iso); \
  Context::Scope context_scope(local_ctx);

MaybeLocal<Module> ResolveModuleCallback(Local<Context> context,
                                         Local<String> specifier,
                                         Local<FixedArray>,
                                         Local<Module> referrer) {
  Isolate* iso = Isolate::GetCurrent();
  String::Utf8Value spec(iso, specifier);

  gv8_ctx* ctx = static_cast<gv8_ctx*>(
      Local<External>::Cast(context->GetEmbedderDataV2(2))->Value(kContextWrapTag));
  gv8_module ref;
  ref.iso = iso;
  ref.ptr.Reset(iso, referrer);

  GV8ResolvedModule resolved =
      gv8ResolveModuleCallback(ctx->ref, const_cast<char*>(*spec), &ref);
  if (resolved.error_value != nullptr) {
    iso->ThrowException(resolved.error_value->ptr.Get(iso));
    return MaybeLocal<Module>();
  }
  if (resolved.module == nullptr) {
    return MaybeLocal<Module>();
  }
  return resolved.module->ptr.Get(iso);
}

MaybeLocal<Promise> DynamicImportCallback(Local<Context> context,
                                          Local<Data>,
                                          Local<Value> resource_name,
                                          Local<String> specifier,
                                          Local<FixedArray>) {
  Isolate* iso = Isolate::GetCurrent();
  String::Utf8Value spec(iso, specifier);
  String::Utf8Value resource(iso, resource_name);

  gv8_ctx* ctx = static_cast<gv8_ctx*>(
      Local<External>::Cast(context->GetEmbedderDataV2(2))->Value(kContextWrapTag));

  GV8DynamicImportResult resolved = gv8ResolveDynamicImportCallback(
      ctx->ref, const_cast<char*>(*spec), const_cast<char*>(*resource));
  if (resolved.error_value != nullptr) {
    iso->ThrowException(resolved.error_value->ptr.Get(iso));
    return MaybeLocal<Promise>();
  }
  if (resolved.promise == nullptr) {
    return MaybeLocal<Promise>();
  }
  return resolved.promise->ptr.Get(iso).As<Promise>();
}

void HostFunctionCallback(const FunctionCallbackInfo<Value>& info) {
  Isolate* iso = info.GetIsolate();
  HandleScope handle_scope(iso);
  Local<Context> context = iso->GetCurrentContext();
  gv8_ctx* ctx = static_cast<gv8_ctx*>(
      Local<External>::Cast(context->GetEmbedderDataV2(2))->Value(kContextWrapTag));
  int callback_id = info.Data().As<Int32>()->Value();

  gv8_value* recv = wrapValue(ctx, iso, info.This());
  GV8ValuePtr* argv = nullptr;
  if (info.Length() > 0) {
    argv = static_cast<GV8ValuePtr*>(malloc(sizeof(GV8ValuePtr) * info.Length()));
    for (int i = 0; i < info.Length(); ++i) {
      argv[i] = wrapValue(ctx, iso, info[i]);
    }
  }

  GV8CallbackResult result =
      gv8FunctionCallback(ctx->ref, callback_id, recv, info.Length(), argv);
  if (argv != nullptr) {
    free(argv);
  }

  if (result.error_value != nullptr) {
    iso->ThrowException(result.error_value->ptr.Get(iso));
    return;
  }
  if (result.value != nullptr) {
    info.GetReturnValue().Set(result.value->ptr.Get(iso));
  }
}

void OnPromiseReject(PromiseRejectMessage message) {
  Isolate* iso = Isolate::GetCurrent();
  intptr_t ref = reinterpret_cast<intptr_t>(iso->GetData(1));
  if (ref == 0) {
    return;
  }

  HandleScope handle_scope(iso);
  Local<Context> ctx = iso->GetEnteredOrMicrotaskContext();
  Local<Message> msg = Exception::CreateMessage(iso, message.GetValue());
  GV8RtnError error = ErrorFromMessage(iso, ctx, msg, message.GetValue());
  gv8PromiseRejectCallback(static_cast<int>(ref),
                           static_cast<int>(message.GetEvent()),
                           error);
}

void ExceptionMessageCallback(Local<Message> message, Local<Value> data) {
  Isolate* iso = Isolate::GetCurrent();
  int ref = 0;
  if (!data.IsEmpty() && data->IsInt32()) {
    ref = data.As<Int32>()->Value();
  }
  if (ref == 0) {
    return;
  }

  HandleScope handle_scope(iso);
  Local<Context> ctx = iso->GetEnteredOrMicrotaskContext();
  GV8RtnError error = ErrorFromMessage(iso, ctx, message, Local<Value>());
  gv8ExceptionMessageCallback(ref, error);
}

extern "C" {

void GV8Init() {
  std::string icu_data = SharedLibraryDir() + "/icudtl.dat";
  V8::InitializeICUDefaultLocation(nullptr, icu_data.c_str());
  V8::InitializePlatform(default_platform.get());
  V8::Initialize();
  default_allocator = ArrayBuffer::Allocator::NewDefaultAllocator();
}

GV8IsolatePtr GV8NewIsolate() {
  Isolate::CreateParams params;
  params.array_buffer_allocator = default_allocator;
  Isolate* iso = Isolate::New(params);
  iso->SetHostImportModuleDynamicallyCallback(DynamicImportCallback);
  ISO_SCOPE(iso)

  gv8_ctx* internal = new gv8_ctx;
  internal->iso = iso;
  internal->ref = 0;
  internal->next_value_id = 0;
  internal->ptr.Reset(iso, Context::New(iso));
  iso->SetData(0, internal);
  iso->SetPromiseRejectCallback(OnPromiseReject);
  return iso;
}

GV8IsolatePtr GV8NewIsolateWithHeapLimit(uint64_t initial_heap_size,
                                         uint64_t max_heap_size) {
  Isolate::CreateParams params;
  params.array_buffer_allocator = default_allocator;
  if (initial_heap_size > 0 || max_heap_size > 0) {
    params.constraints.ConfigureDefaultsFromHeapSize(
        static_cast<size_t>(initial_heap_size), static_cast<size_t>(max_heap_size));
  }
  Isolate* iso = Isolate::New(params);
  iso->SetHostImportModuleDynamicallyCallback(DynamicImportCallback);
  ISO_SCOPE(iso)

  gv8_ctx* internal = new gv8_ctx;
  internal->iso = iso;
  internal->ref = 0;
  internal->next_value_id = 0;
  internal->ptr.Reset(iso, Context::New(iso));
  iso->SetData(0, internal);
  iso->SetPromiseRejectCallback(OnPromiseReject);
  return iso;
}

void GV8IsolateSetObserverRef(GV8IsolatePtr iso, int ref) {
  if (iso == nullptr) {
    return;
  }
  iso->SetData(1, reinterpret_cast<void*>(static_cast<intptr_t>(ref)));
}

void GV8IsolateDispose(GV8IsolatePtr iso) {
  if (iso == nullptr) {
    return;
  }
  GV8ContextDispose(internalContext(iso));
  iso->Dispose();
}

void GV8IsolatePerformMicrotaskCheckpoint(GV8IsolatePtr iso) {
  ISO_SCOPE(iso)
  iso->PerformMicrotaskCheckpoint();
}

void GV8IsolateTerminateExecution(GV8IsolatePtr iso) {
  if (iso == nullptr) {
    return;
  }
  iso->TerminateExecution();
}

int GV8IsolateIsExecutionTerminating(GV8IsolatePtr iso) {
  if (iso == nullptr) {
    return 0;
  }
  return iso->IsExecutionTerminating() ? 1 : 0;
}

void GV8IsolateCancelTerminateExecution(GV8IsolatePtr iso) {
  if (iso == nullptr) {
    return;
  }
  iso->CancelTerminateExecution();
}

void GV8IsolateLowMemoryNotification(GV8IsolatePtr iso) {
  if (iso == nullptr) {
    return;
  }
  ISO_SCOPE(iso)
  iso->LowMemoryNotification();
}

void GV8IsolateMemoryPressureNotification(GV8IsolatePtr iso, int level) {
  if (iso == nullptr) {
    return;
  }
  ISO_SCOPE(iso)
  MemoryPressureLevel pressure = MemoryPressureLevel::kNone;
  switch (level) {
    case 1:
      pressure = MemoryPressureLevel::kModerate;
      break;
    case 2:
      pressure = MemoryPressureLevel::kCritical;
      break;
    default:
      break;
  }
  iso->MemoryPressureNotification(pressure);
}

GV8HeapStatistics GV8IsolateGetHeapStatistics(GV8IsolatePtr iso) {
  GV8HeapStatistics rtn = {};
  if (iso == nullptr) {
    return rtn;
  }
  ISO_SCOPE(iso)
  HeapStatistics stats;
  iso->GetHeapStatistics(&stats);
  rtn.total_heap_size = static_cast<uint64_t>(stats.total_heap_size());
  rtn.total_heap_size_executable =
      static_cast<uint64_t>(stats.total_heap_size_executable());
  rtn.total_physical_size = static_cast<uint64_t>(stats.total_physical_size());
  rtn.total_available_size = static_cast<uint64_t>(stats.total_available_size());
  rtn.total_global_handles_size =
      static_cast<uint64_t>(stats.total_global_handles_size());
  rtn.used_global_handles_size =
      static_cast<uint64_t>(stats.used_global_handles_size());
  rtn.used_heap_size = static_cast<uint64_t>(stats.used_heap_size());
  rtn.heap_size_limit = static_cast<uint64_t>(stats.heap_size_limit());
  rtn.malloced_memory = static_cast<uint64_t>(stats.malloced_memory());
  rtn.external_memory = static_cast<uint64_t>(stats.external_memory());
  rtn.peak_malloced_memory = static_cast<uint64_t>(stats.peak_malloced_memory());
  rtn.number_of_native_contexts =
      static_cast<uint64_t>(stats.number_of_native_contexts());
  rtn.number_of_detached_contexts =
      static_cast<uint64_t>(stats.number_of_detached_contexts());
  rtn.total_allocated_bytes =
      static_cast<uint64_t>(stats.total_allocated_bytes());
  return rtn;
}

GV8ContextPtr GV8NewContext(GV8IsolatePtr iso, int ref) {
  ISO_SCOPE(iso)

  Local<Context> ctx = Context::New(iso);
  ctx->SetEmbedderDataV2(1, Integer::New(iso, ref));

  gv8_ctx* wrap = new gv8_ctx;
  wrap->iso = iso;
  wrap->ref = ref;
  wrap->next_value_id = 0;
  wrap->ptr.Reset(iso, ctx);
  ctx->SetEmbedderDataV2(2, External::New(iso, wrap, kContextWrapTag));
  return wrap;
}

void GV8ContextDispose(GV8ContextPtr ctx) {
  if (ctx == nullptr) {
    return;
  }
  ctx->ptr.Reset();
  for (auto it = ctx->values.begin(); it != ctx->values.end(); ++it) {
    it->second->ptr.Reset();
    delete it->second;
  }
  ctx->values.clear();
  delete ctx;
}

GV8ValuePtr GV8ContextGlobal(GV8ContextPtr ctx_ptr) {
  CTX_SCOPE(ctx_ptr)
  return wrapValue(ctx_ptr, iso, local_ctx->Global());
}

GV8RtnValue GV8ContextNewObject(GV8ContextPtr ctx_ptr) {
  CTX_SCOPE(ctx_ptr)
  GV8RtnValue rtn = {};
  rtn.value = wrapValue(ctx_ptr, iso, Object::New(iso));
  return rtn;
}

GV8RtnValue GV8ContextNewFunction(GV8ContextPtr ctx_ptr, int callback_id) {
  CTX_SCOPE(ctx_ptr)
  GV8RtnValue rtn = {};

  Local<Function> fn;
  if (!Function::New(local_ctx, HostFunctionCallback, Int32::New(iso, callback_id))
           .ToLocal(&fn)) {
    rtn.error = ExceptionError(try_catch, iso, local_ctx);
    return rtn;
  }

  rtn.value = wrapValue(ctx_ptr, iso, fn);
  return rtn;
}

GV8RtnValue GV8ContextNewPromiseResolver(GV8ContextPtr ctx_ptr) {
  CTX_SCOPE(ctx_ptr)
  GV8RtnValue rtn = {};

  Local<Promise::Resolver> resolver;
  if (!Promise::Resolver::New(local_ctx).ToLocal(&resolver)) {
    rtn.error = ExceptionError(try_catch, iso, local_ctx);
    return rtn;
  }

  rtn.value = wrapValue(ctx_ptr, iso, resolver);
  return rtn;
}

GV8RtnValue GV8ContextRunScript(GV8ContextPtr ctx_ptr,
                                const char* source,
                                const char* resource_name,
                                int line_offset,
                                int column_offset) {
  CTX_SCOPE(ctx_ptr)
  GV8RtnValue rtn = {};

  Local<String> src =
      String::NewFromUtf8(iso, source, NewStringType::kNormal).ToLocalChecked();
  ScriptOrigin origin =
      MakeOrigin(iso, resource_name, line_offset, column_offset, false);
  ScriptCompiler::Source script_source(src, origin);

  Local<Script> script;
  if (!ScriptCompiler::Compile(local_ctx, &script_source).ToLocal(&script)) {
    rtn.error = ExceptionError(try_catch, iso, local_ctx);
    return rtn;
  }

  Local<Value> result;
  if (!script->Run(local_ctx).ToLocal(&result)) {
    rtn.error = ExceptionError(try_catch, iso, local_ctx);
    return rtn;
  }

  rtn.value = wrapValue(ctx_ptr, iso, result);
  return rtn;
}

GV8RtnUnboundScript GV8CompileUnboundScript(GV8IsolatePtr iso,
                                            const char* source,
                                            const char* resource_name,
                                            int line_offset,
                                            int column_offset) {
  ISO_SCOPE(iso)
  GV8RtnUnboundScript rtn = {};

  gv8_ctx* internal = internalContext(iso);
  Local<Context> local_ctx = internal->ptr.Get(iso);
  Context::Scope context_scope(local_ctx);
  TryCatch try_catch(iso);

  Local<String> src =
      String::NewFromUtf8(iso, source, NewStringType::kNormal).ToLocalChecked();
  ScriptOrigin origin =
      MakeOrigin(iso, resource_name, line_offset, column_offset, false);
  ScriptCompiler::Source script_source(src, origin);

  Local<UnboundScript> script;
  if (!ScriptCompiler::CompileUnboundScript(iso, &script_source).ToLocal(&script)) {
    rtn.error = ExceptionError(try_catch, iso, local_ctx);
    return rtn;
  }

  gv8_unbound_script* wrap = new gv8_unbound_script;
  wrap->iso = iso;
  wrap->ptr.Reset(iso, script);
  rtn.ptr = wrap;
  return rtn;
}

GV8RtnValue GV8UnboundScriptRun(GV8ContextPtr ctx_ptr,
                                GV8UnboundScriptPtr script_ptr) {
  CTX_SCOPE(ctx_ptr)
  GV8RtnValue rtn = {};

  Local<UnboundScript> script = script_ptr->ptr.Get(iso);
  Local<Script> bound = script->BindToCurrentContext();

  Local<Value> result;
  if (!bound->Run(local_ctx).ToLocal(&result)) {
    rtn.error = ExceptionError(try_catch, iso, local_ctx);
    return rtn;
  }

  rtn.value = wrapValue(ctx_ptr, iso, result);
  return rtn;
}

void GV8UnboundScriptRelease(GV8UnboundScriptPtr script_ptr) {
  if (script_ptr == nullptr) {
    return;
  }
  script_ptr->ptr.Reset();
  delete script_ptr;
}

GV8RtnModule GV8CompileModule(GV8IsolatePtr iso,
                              const char* source,
                              const char* resource_name,
                              int line_offset,
                              int column_offset) {
  ISO_SCOPE(iso)
  GV8RtnModule rtn = {};

  gv8_ctx* internal = internalContext(iso);
  Local<Context> local_ctx = internal->ptr.Get(iso);
  Context::Scope context_scope(local_ctx);
  TryCatch try_catch(iso);

  Local<String> src =
      String::NewFromUtf8(iso, source, NewStringType::kNormal).ToLocalChecked();
  ScriptOrigin origin =
      MakeOrigin(iso, resource_name, line_offset, column_offset, true);
  ScriptCompiler::Source script_source(src, origin);

  Local<Module> module;
  if (!ScriptCompiler::CompileModule(iso, &script_source).ToLocal(&module)) {
    rtn.error = ExceptionError(try_catch, iso, local_ctx);
    return rtn;
  }

  gv8_module* wrap = new gv8_module;
  wrap->iso = iso;
  wrap->ptr.Reset(iso, module);
  rtn.ptr = wrap;
  return rtn;
}

void GV8ModuleRelease(GV8ModulePtr module_ptr) {
  if (module_ptr == nullptr) {
    return;
  }
  module_ptr->ptr.Reset();
  delete module_ptr;
}

GV8RtnError GV8ModuleInstantiate(GV8ContextPtr ctx_ptr, GV8ModulePtr module_ptr) {
  CTX_SCOPE(ctx_ptr)
  GV8RtnError rtn = {nullptr, nullptr, nullptr};

  Local<Module> module = module_ptr->ptr.Get(iso);
  Maybe<bool> ok = module->InstantiateModule(local_ctx, ResolveModuleCallback);
  if (ok.IsNothing()) {
    rtn = ExceptionError(try_catch, iso, local_ctx);
  }
  return rtn;
}

GV8RtnValue GV8ModuleEvaluate(GV8ContextPtr ctx_ptr, GV8ModulePtr module_ptr) {
  CTX_SCOPE(ctx_ptr)
  GV8RtnValue rtn = {};

  Local<Module> module = module_ptr->ptr.Get(iso);
  Local<Value> result;
  if (!module->Evaluate(local_ctx).ToLocal(&result)) {
    rtn.error = ExceptionError(try_catch, iso, local_ctx);
    return rtn;
  }

  rtn.value = wrapValue(ctx_ptr, iso, result);
  return rtn;
}

GV8ValuePtr GV8ModuleGetNamespace(GV8ContextPtr ctx_ptr, GV8ModulePtr module_ptr) {
  CTX_SCOPE(ctx_ptr)

  Local<Module> module = module_ptr->ptr.Get(iso);
  Local<Value> result = module->GetModuleNamespace();

  return wrapValue(ctx_ptr, iso, result);
}

int GV8ModuleGetStatus(GV8ModulePtr module_ptr) {
  Isolate* iso = module_ptr->iso;
  ISO_SCOPE(iso)

  Local<Module> module = module_ptr->ptr.Get(iso);
  return static_cast<int>(module->GetStatus());
}

int GV8ModuleGetScriptID(GV8ModulePtr module_ptr) {
  Isolate* iso = module_ptr->iso;
  ISO_SCOPE(iso)

  Local<Module> module = module_ptr->ptr.Get(iso);
  return module->ScriptId();
}

GV8RtnValue GV8NewString(GV8ContextPtr ctx_ptr, const char* value, int length) {
  CTX_SCOPE(ctx_ptr)
  GV8RtnValue rtn = {};

  Local<String> str;
  if (!String::NewFromUtf8(iso, value, NewStringType::kNormal, length).ToLocal(&str)) {
    rtn.error = ExceptionError(try_catch, iso, local_ctx);
    return rtn;
  }

  rtn.value = wrapValue(ctx_ptr, iso, str);
  return rtn;
}

GV8RtnValue GV8NewBoolean(GV8ContextPtr ctx_ptr, int value) {
  CTX_SCOPE(ctx_ptr)
  GV8RtnValue rtn = {};
  rtn.value = wrapValue(ctx_ptr, iso, Boolean::New(iso, value != 0));
  return rtn;
}

GV8RtnValue GV8NewInteger(GV8ContextPtr ctx_ptr, int64_t value) {
  CTX_SCOPE(ctx_ptr)
  GV8RtnValue rtn = {};
  rtn.value = wrapValue(ctx_ptr, iso, Number::New(iso, static_cast<double>(value)));
  return rtn;
}

GV8RtnValue GV8NewNull(GV8ContextPtr ctx_ptr) {
  CTX_SCOPE(ctx_ptr)
  GV8RtnValue rtn = {};
  rtn.value = wrapValue(ctx_ptr, iso, Null(iso));
  return rtn;
}

GV8RtnValue GV8NewUndefined(GV8ContextPtr ctx_ptr) {
  CTX_SCOPE(ctx_ptr)
  GV8RtnValue rtn = {};
  rtn.value = wrapValue(ctx_ptr, iso, Undefined(iso));
  return rtn;
}

GV8RtnValue GV8NewError(GV8ContextPtr ctx_ptr, const char* value, int length) {
  CTX_SCOPE(ctx_ptr)
  GV8RtnValue rtn = {};
  std::string msg(value, length);
  rtn.value = wrapValue(ctx_ptr, iso, NewError(iso, local_ctx, msg));
  return rtn;
}

void GV8ValueRelease(GV8ValuePtr value) {
  if (value == nullptr) {
    return;
  }
  gv8_ctx* ctx = value->ctx;
  if (ctx != nullptr && value->id != 0) {
    ctx->values.erase(value->id);
  }
  value->ptr.Reset();
  delete value;
}

GV8RtnString GV8ValueToString(GV8ValuePtr value_ptr) {
  GV8RtnString rtn = {};
  if (value_ptr == nullptr) {
    return rtn;
  }

  Isolate* iso = value_ptr->iso;
  ISO_SCOPE(iso)
  TryCatch try_catch(iso);
  Local<Context> local_ctx = value_ptr->ctx->ptr.Get(iso);
  Context::Scope context_scope(local_ctx);

  String::Utf8Value utf8(iso, value_ptr->ptr.Get(iso));
  if (*utf8 == nullptr) {
    rtn.error = ExceptionError(try_catch, iso, local_ctx);
    return rtn;
  }
  rtn.data = CopyString(utf8);
  rtn.length = static_cast<int>(utf8.length());
  return rtn;
}

int64_t GV8ValueToInteger(GV8ValuePtr value_ptr) {
  Isolate* iso = value_ptr->iso;
  ISO_SCOPE(iso)
  Local<Context> local_ctx = value_ptr->ctx->ptr.Get(iso);
  Context::Scope context_scope(local_ctx);
  return value_ptr->ptr.Get(iso)->IntegerValue(local_ctx).FromMaybe(0);
}

int GV8ValueToBoolean(GV8ValuePtr value_ptr) {
  Isolate* iso = value_ptr->iso;
  ISO_SCOPE(iso)
  Local<Context> local_ctx = value_ptr->ctx->ptr.Get(iso);
  Context::Scope context_scope(local_ctx);
  return value_ptr->ptr.Get(iso)->BooleanValue(iso) ? 1 : 0;
}

int GV8ValueIsObject(GV8ValuePtr value_ptr) {
  Isolate* iso = value_ptr->iso;
  ISO_SCOPE(iso)
  return value_ptr->ptr.Get(iso)->IsObject() ? 1 : 0;
}

int GV8ValueIsString(GV8ValuePtr value_ptr) {
  Isolate* iso = value_ptr->iso;
  ISO_SCOPE(iso)
  return value_ptr->ptr.Get(iso)->IsString() ? 1 : 0;
}

int GV8ValueIsBoolean(GV8ValuePtr value_ptr) {
  Isolate* iso = value_ptr->iso;
  ISO_SCOPE(iso)
  return value_ptr->ptr.Get(iso)->IsBoolean() ? 1 : 0;
}

int GV8ValueIsUndefined(GV8ValuePtr value_ptr) {
  Isolate* iso = value_ptr->iso;
  ISO_SCOPE(iso)
  return value_ptr->ptr.Get(iso)->IsUndefined() ? 1 : 0;
}

int GV8ValueIsNull(GV8ValuePtr value_ptr) {
  Isolate* iso = value_ptr->iso;
  ISO_SCOPE(iso)
  return value_ptr->ptr.Get(iso)->IsNull() ? 1 : 0;
}

int GV8ValueIsPromise(GV8ValuePtr value_ptr) {
  Isolate* iso = value_ptr->iso;
  ISO_SCOPE(iso)
  return value_ptr->ptr.Get(iso)->IsPromise() ? 1 : 0;
}

int GV8ValueIsFunction(GV8ValuePtr value_ptr) {
  Isolate* iso = value_ptr->iso;
  ISO_SCOPE(iso)
  return value_ptr->ptr.Get(iso)->IsFunction() ? 1 : 0;
}

int GV8ValueIsArrayBuffer(GV8ValuePtr value_ptr) {
  Isolate* iso = value_ptr->iso;
  ISO_SCOPE(iso)
  return value_ptr->ptr.Get(iso)->IsArrayBuffer() ? 1 : 0;
}

int GV8ValueIsArrayBufferView(GV8ValuePtr value_ptr) {
  Isolate* iso = value_ptr->iso;
  ISO_SCOPE(iso)
  return value_ptr->ptr.Get(iso)->IsArrayBufferView() ? 1 : 0;
}

int GV8ValueIsUint8Array(GV8ValuePtr value_ptr) {
  Isolate* iso = value_ptr->iso;
  ISO_SCOPE(iso)
  return value_ptr->ptr.Get(iso)->IsUint8Array() ? 1 : 0;
}

int GV8ValueIsInt8Array(GV8ValuePtr value_ptr) {
  Isolate* iso = value_ptr->iso;
  ISO_SCOPE(iso)
  return value_ptr->ptr.Get(iso)->IsInt8Array() ? 1 : 0;
}

int GV8ValueIsUint8ClampedArray(GV8ValuePtr value_ptr) {
  Isolate* iso = value_ptr->iso;
  ISO_SCOPE(iso)
  return value_ptr->ptr.Get(iso)->IsUint8ClampedArray() ? 1 : 0;
}

GV8RtnBytes GV8ValueBytes(GV8ValuePtr value_ptr) {
  GV8RtnBytes rtn = {};
  Isolate* iso = value_ptr->iso;
  ISO_SCOPE(iso)
  TryCatch try_catch(iso);
  Local<Context> local_ctx = value_ptr->ctx->ptr.Get(iso);
  Context::Scope context_scope(local_ctx);
  Local<Value> value = value_ptr->ptr.Get(iso);

  std::shared_ptr<BackingStore> backing_store;
  size_t offset = 0;
  size_t length = 0;

  if (value->IsArrayBuffer()) {
    Local<ArrayBuffer> buffer = value.As<ArrayBuffer>();
    backing_store = buffer->GetBackingStore();
    length = backing_store->ByteLength();
  } else if (value->IsArrayBufferView()) {
    Local<ArrayBufferView> view = value.As<ArrayBufferView>();
    backing_store = view->Buffer()->GetBackingStore();
    offset = view->ByteOffset();
    length = view->ByteLength();
  } else {
    rtn.error = EmptyError();
    rtn.error.msg = CopyString("gv8: value is not an ArrayBuffer or ArrayBufferView");
    return rtn;
  }

  uint8_t* copy = static_cast<uint8_t*>(malloc(length));
  if (length > 0) {
    memcpy(copy, static_cast<uint8_t*>(backing_store->Data()) + offset, length);
  }
  rtn.data = copy;
  rtn.length = static_cast<int>(length);
  return rtn;
}

GV8RtnValue GV8ObjectGet(GV8ValuePtr value_ptr, const char* key) {
  Isolate* iso = value_ptr->iso;
  ISO_SCOPE(iso)
  TryCatch try_catch(iso);
  GV8RtnValue rtn = {};

  Local<Context> local_ctx = value_ptr->ctx->ptr.Get(iso);
  Context::Scope context_scope(local_ctx);
  Local<Object> object = value_ptr->ptr.Get(iso).As<Object>();
  Local<String> prop =
      String::NewFromUtf8(iso, key, NewStringType::kNormal).ToLocalChecked();

  Local<Value> result;
  if (!object->Get(local_ctx, prop).ToLocal(&result)) {
    rtn.error = ExceptionError(try_catch, iso, local_ctx);
    return rtn;
  }

  rtn.value = wrapValue(value_ptr->ctx, iso, result);
  return rtn;
}

GV8RtnValue GV8ObjectGetIdx(GV8ValuePtr value_ptr, uint32_t index) {
  Isolate* iso = value_ptr->iso;
  ISO_SCOPE(iso)
  TryCatch try_catch(iso);
  GV8RtnValue rtn = {};

  Local<Context> local_ctx = value_ptr->ctx->ptr.Get(iso);
  Context::Scope context_scope(local_ctx);
  Local<Object> object = value_ptr->ptr.Get(iso).As<Object>();

  Local<Value> result;
  if (!object->Get(local_ctx, index).ToLocal(&result)) {
    rtn.error = ExceptionError(try_catch, iso, local_ctx);
    return rtn;
  }

  rtn.value = wrapValue(value_ptr->ctx, iso, result);
  return rtn;
}

GV8RtnError GV8ObjectSet(GV8ValuePtr value_ptr, const char* key, GV8ValuePtr prop_ptr) {
  Isolate* iso = value_ptr->iso;
  ISO_SCOPE(iso)
  TryCatch try_catch(iso);

  Local<Context> local_ctx = value_ptr->ctx->ptr.Get(iso);
  Context::Scope context_scope(local_ctx);
  Local<Object> object = value_ptr->ptr.Get(iso).As<Object>();
  Local<String> prop =
      String::NewFromUtf8(iso, key, NewStringType::kNormal).ToLocalChecked();

  if (object->Set(local_ctx, prop, prop_ptr->ptr.Get(iso)).IsNothing()) {
    return ExceptionError(try_catch, iso, local_ctx);
  }
  return EmptyError();
}

GV8RtnValue GV8ValueCall(GV8ValuePtr value_ptr,
                         GV8ValuePtr recv_ptr,
                         int argc,
                         GV8ValuePtr* argv) {
  Isolate* iso = value_ptr->iso;
  ISO_SCOPE(iso)
  TryCatch try_catch(iso);
  GV8RtnValue rtn = {};

  Local<Context> local_ctx = value_ptr->ctx->ptr.Get(iso);
  Context::Scope context_scope(local_ctx);
  Local<Function> fn = value_ptr->ptr.Get(iso).As<Function>();
  std::vector<Local<Value>> args;
  args.reserve(argc);
  for (int i = 0; i < argc; ++i) {
    args.push_back(argv[i]->ptr.Get(iso));
  }

  Local<Value> result;
  if (!fn->Call(local_ctx, recv_ptr->ptr.Get(iso), argc, args.data()).ToLocal(&result)) {
    rtn.error = ExceptionError(try_catch, iso, local_ctx);
    return rtn;
  }
  rtn.value = wrapValue(value_ptr->ctx, iso, result);
  return rtn;
}

GV8RtnValue GV8ObjectCallMethod(GV8ValuePtr value_ptr,
                                const char* key,
                                int argc,
                                GV8ValuePtr* argv) {
  GV8RtnValue method_rtn = GV8ObjectGet(value_ptr, key);
  if (method_rtn.value == nullptr) {
    return method_rtn;
  }
  return GV8ValueCall(method_rtn.value, value_ptr, argc, argv);
}

int GV8PromiseState(GV8ValuePtr value_ptr) {
  Isolate* iso = value_ptr->iso;
  ISO_SCOPE(iso)
  Local<Promise> promise = value_ptr->ptr.Get(iso).As<Promise>();
  return static_cast<int>(promise->State());
}

GV8ValuePtr GV8PromiseResult(GV8ValuePtr value_ptr) {
  Isolate* iso = value_ptr->iso;
  ISO_SCOPE(iso)
  Local<Context> local_ctx = value_ptr->ctx->ptr.Get(iso);
  Context::Scope context_scope(local_ctx);
  Local<Promise> promise = value_ptr->ptr.Get(iso).As<Promise>();
  return wrapValue(value_ptr->ctx, iso, promise->Result());
}

GV8ValuePtr GV8PromiseResolverGetPromise(GV8ValuePtr value_ptr) {
  Isolate* iso = value_ptr->iso;
  ISO_SCOPE(iso)
  Local<Context> local_ctx = value_ptr->ctx->ptr.Get(iso);
  Context::Scope context_scope(local_ctx);
  Local<Promise::Resolver> resolver =
      value_ptr->ptr.Get(iso).As<Promise::Resolver>();
  return wrapValue(value_ptr->ctx, iso, resolver->GetPromise());
}

GV8RtnError GV8PromiseResolverResolve(GV8ValuePtr value_ptr, GV8ValuePtr result_ptr) {
  Isolate* iso = value_ptr->iso;
  ISO_SCOPE(iso)
  TryCatch try_catch(iso);

  Local<Context> local_ctx = value_ptr->ctx->ptr.Get(iso);
  Context::Scope context_scope(local_ctx);
  Local<Promise::Resolver> resolver =
      value_ptr->ptr.Get(iso).As<Promise::Resolver>();
  if (resolver->Resolve(local_ctx, result_ptr->ptr.Get(iso)).IsNothing()) {
    return ExceptionError(try_catch, iso, local_ctx);
  }
  return EmptyError();
}

GV8RtnError GV8PromiseResolverReject(GV8ValuePtr value_ptr, GV8ValuePtr result_ptr) {
  Isolate* iso = value_ptr->iso;
  ISO_SCOPE(iso)
  TryCatch try_catch(iso);

  Local<Context> local_ctx = value_ptr->ctx->ptr.Get(iso);
  Context::Scope context_scope(local_ctx);
  Local<Promise::Resolver> resolver =
      value_ptr->ptr.Get(iso).As<Promise::Resolver>();
  if (resolver->Reject(local_ctx, result_ptr->ptr.Get(iso)).IsNothing()) {
    return ExceptionError(try_catch, iso, local_ctx);
  }
  return EmptyError();
}

GV8RtnString GV8JSONStringify(GV8ContextPtr ctx_ptr, GV8ValuePtr value_ptr) {
  CTX_SCOPE(ctx_ptr)
  GV8RtnString rtn = {};

  Local<String> result;
  if (!JSON::Stringify(local_ctx, value_ptr->ptr.Get(iso)).ToLocal(&result)) {
    rtn.error = ExceptionError(try_catch, iso, local_ctx);
    return rtn;
  }

  String::Utf8Value utf8(iso, result);
  rtn.data = CopyString(utf8);
  rtn.length = static_cast<int>(utf8.length());
  return rtn;
}

GV8RtnValue GV8JSONParse(GV8ContextPtr ctx_ptr, const char* value, int length) {
  CTX_SCOPE(ctx_ptr)
  GV8RtnValue rtn = {};

  Local<String> json;
  if (!String::NewFromUtf8(iso, value, NewStringType::kNormal, length).ToLocal(&json)) {
    rtn.error = ExceptionError(try_catch, iso, local_ctx);
    return rtn;
  }

  Local<Value> result;
  if (!JSON::Parse(local_ctx, json).ToLocal(&result)) {
    rtn.error = ExceptionError(try_catch, iso, local_ctx);
    return rtn;
  }

  rtn.value = wrapValue(ctx_ptr, iso, result);
  return rtn;
}

}  // extern "C"
