//go:build cgo && (linux || darwin)

#include "bridge.h"

#include <v8.h>
#include <libplatform/libplatform.h>

#include <climits>
#include <cstdlib>
#include <cstring>
#include <memory>
#include <mutex>
#include <string>

struct gone_v8_vm {
  std::unique_ptr<v8::ArrayBuffer::Allocator> allocator;
  v8::Isolate* isolate = nullptr;
  v8::Global<v8::Context> context;
};

namespace {
std::once_flag init_once;
std::unique_ptr<v8::Platform> platform;

void init_v8() {
  platform = v8::platform::NewDefaultPlatform();
  v8::V8::InitializePlatform(platform.get());
  v8::V8::Initialize();
}

gone_v8_result make_result(const std::string& text, bool is_error) {
  char* data = static_cast<char*>(std::malloc(text.size() + 1));
  if (data == nullptr) return {nullptr, 0, 1};
  if (!text.empty()) std::memcpy(data, text.data(), text.size());
  data[text.size()] = '\0';
  return {data, text.size(), is_error ? 1 : 0};
}

std::string utf8(v8::Isolate* isolate, v8::Local<v8::String> text) {
  v8::String::Utf8Value value(isolate, text);
  if (*value == nullptr) return "";
  return std::string(*value, value.length());
}

gone_v8_result exception_result(v8::Isolate* isolate,
                                v8::Local<v8::Context> context,
                                v8::TryCatch& catcher) {
  std::string message = "JavaScript execution failed";
  if (catcher.HasCaught() && !catcher.Exception().IsEmpty()) {
    v8::Local<v8::String> converted;
    if (catcher.Exception()->ToString(context).ToLocal(&converted)) {
      message = utf8(isolate, converted);
    }
    auto info = catcher.Message();
    if (!info.IsEmpty()) {
      int line = info->GetLineNumber(context).FromMaybe(0);
      if (line > 0) message += " (line " + std::to_string(line) + ")";
    }
  }
  return make_result(message, true);
}
}  // namespace

extern "C" gone_v8_vm* gone_v8_new(void) {
  std::call_once(init_once, init_v8);
  auto vm = std::make_unique<gone_v8_vm>();
  vm->allocator.reset(v8::ArrayBuffer::Allocator::NewDefaultAllocator());
  v8::Isolate::CreateParams params;
  params.array_buffer_allocator = vm->allocator.get();
  vm->isolate = v8::Isolate::New(params);
  if (!vm->isolate) return nullptr;
  {
    v8::Isolate::Scope isolate_scope(vm->isolate);
    v8::HandleScope handle_scope(vm->isolate);
    auto context = v8::Context::New(vm->isolate);
    vm->context.Reset(vm->isolate, context);
  }
  return vm.release();
}

extern "C" gone_v8_result gone_v8_eval(gone_v8_vm* vm,
                                         const char* source, size_t len) {
  if (!vm || !vm->isolate) return make_result("V8 isolate is closed", true);
  if (len > INT_MAX) return make_result("Source is too large", true);
  v8::Isolate* isolate = vm->isolate;
  v8::Isolate::Scope isolate_scope(isolate);
  v8::HandleScope handle_scope(isolate);
  auto context = vm->context.Get(isolate);
  v8::Context::Scope context_scope(context);
  v8::TryCatch catcher(isolate);

  v8::Local<v8::String> code;
  if (!v8::String::NewFromUtf8(isolate, source, v8::NewStringType::kNormal,
                               static_cast<int>(len)).ToLocal(&code)) {
    return exception_result(isolate, context, catcher);
  }
  v8::Local<v8::Script> script;
  if (!v8::Script::Compile(context, code).ToLocal(&script)) {
    return exception_result(isolate, context, catcher);
  }
  v8::Local<v8::Value> value;
  if (!script->Run(context).ToLocal(&value)) {
    return exception_result(isolate, context, catcher);
  }
  v8::Local<v8::String> text;
  if (!value->ToString(context).ToLocal(&text)) {
    return exception_result(isolate, context, catcher);
  }
  return make_result(utf8(isolate, text), false);
}

extern "C" void gone_v8_result_free(gone_v8_result* result) {
  if (!result) return;
  std::free(result->data);
  result->data = nullptr;
  result->length = 0;
}

extern "C" void gone_v8_free(gone_v8_vm* vm) {
  if (!vm) return;
  {
    v8::Isolate::Scope isolate_scope(vm->isolate);
    vm->context.Reset();
  }
  vm->isolate->Dispose();
  delete vm;
  // Do not call V8::Dispose here. Other isolates may still exist.
}

#include "bridge_lowlevel.h"
