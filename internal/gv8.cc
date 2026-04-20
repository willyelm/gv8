// Copyright 2026 The gv8 authors.

#include "gv8.h"

#include <cstdlib>
#include <cstring>
#include <dlfcn.h>
#include <sstream>
#include <string>
#include <unordered_map>

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
  ISO_SCOPE(iso)

  gv8_ctx* internal = new gv8_ctx;
  internal->iso = iso;
  internal->ref = 0;
  internal->next_value_id = 0;
  internal->ptr.Reset(iso, Context::New(iso));
  iso->SetData(0, internal);
  return iso;
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

}  // extern "C"
