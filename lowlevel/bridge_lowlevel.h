// Implementation included by bridge.cc so both native build paths share it.
#ifndef GONE_V8_LOWLEVEL_IMPL_H
#define GONE_V8_LOWLEVEL_IMPL_H

#include <vector>

namespace {
thread_local gone_v8_scope* current_low_scope = nullptr;
}

struct gone_v8_scope {
  gone_v8_vm* vm;
  gone_v8_scope* previous;
  v8::Isolate::Scope isolate_scope;
  v8::HandleScope handle_scope;
  v8::Local<v8::Context> context;
  v8::Context::Scope context_scope;
  std::vector<v8::Local<v8::Value>> values;

  explicit gone_v8_scope(gone_v8_vm* owner)
      : vm(owner), previous(current_low_scope), isolate_scope(owner->isolate),
        handle_scope(owner->isolate), context(owner->context.Get(owner->isolate)),
        context_scope(context) { current_low_scope = this; }

  ~gone_v8_scope() { current_low_scope = previous; }

  uintptr_t keep(v8::Local<v8::Value> value) {
    values.push_back(value);
    return values.size();
  }
  v8::Local<v8::Value> get(uintptr_t id) {
    if (id == 0 || id > values.size()) return {};
    return values[id - 1];
  }
};

struct gone_v8_script {
  gone_v8_vm* vm;
  v8::Global<v8::Script> script;
  gone_v8_script(gone_v8_vm* owner, v8::Local<v8::Script> value)
      : vm(owner), script(owner->isolate, value) {}
};

namespace {
v8::MaybeLocal<v8::Script> compile_local(gone_v8_scope* scope,
                                        const char* source, size_t length,
                                        gone_v8_result* error) {
  if (length > INT_MAX) {
    *error = make_result("Source is too large", true);
    return {};
  }
  auto isolate = scope->vm->isolate;
  v8::TryCatch catcher(isolate);
  v8::Local<v8::String> code;
  v8::Local<v8::Script> script;
  if (!v8::String::NewFromUtf8(isolate, source, v8::NewStringType::kNormal,
                              static_cast<int>(length)).ToLocal(&code) ||
      !v8::Script::Compile(scope->context, code).ToLocal(&script)) {
    *error = exception_result(isolate, scope->context, catcher);
    return {};
  }
  return script;
}
}  // namespace

extern "C" uintptr_t gone_v8_thread_id(void) {
  static thread_local char identity;
  return reinterpret_cast<uintptr_t>(&identity);
}

extern "C" gone_v8_scope* gone_v8_scope_open(gone_v8_vm* vm) {
  return new gone_v8_scope(vm);
}
extern "C" void gone_v8_scope_close(gone_v8_scope* scope) { delete scope; }
extern "C" int gone_v8_scope_is_current(gone_v8_scope* scope) {
  return current_low_scope == scope;
}
extern "C" int gone_v8_scope_check(gone_v8_scope* scope, uintptr_t owner_thread) {
  if (gone_v8_thread_id() != owner_thread) return -1;
  return current_low_scope == scope ? 1 : 0;
}

extern "C" gone_v8_script* gone_v8_compile(gone_v8_scope* scope,
    const char* source, size_t length, gone_v8_result* error) {
  *error = {nullptr, 0, 0};
  v8::Local<v8::Script> script;
  if (!compile_local(scope, source, length, error).ToLocal(&script)) return nullptr;
  return new gone_v8_script(scope->vm, script);
}

extern "C" void gone_v8_script_free(gone_v8_script* script) {
  v8::Isolate::Scope entered(script->vm->isolate);
  delete script;
}

extern "C" uintptr_t gone_v8_script_run(gone_v8_scope* scope,
    gone_v8_script* script, gone_v8_result* error) {
  *error = {nullptr, 0, 0};
  if (scope->vm != script->vm) {
    *error = make_result("Script belongs to another isolate", true);
    return 0;
  }
  auto isolate = scope->vm->isolate;
  v8::TryCatch catcher(isolate);
  v8::Local<v8::Value> value;
  if (!script->script.Get(isolate)->Run(scope->context).ToLocal(&value)) {
    *error = exception_result(isolate, scope->context, catcher);
    return 0;
  }
  return scope->keep(value);
}

extern "C" uintptr_t gone_v8_scope_eval(gone_v8_scope* scope,
    const char* source, size_t length, gone_v8_result* error) {
  *error = {nullptr, 0, 0};
  v8::Local<v8::Script> script;
  if (!compile_local(scope, source, length, error).ToLocal(&script)) return 0;
  v8::TryCatch catcher(scope->vm->isolate);
  v8::Local<v8::Value> value;
  if (!script->Run(scope->context).ToLocal(&value)) {
    *error = exception_result(scope->vm->isolate, scope->context, catcher);
    return 0;
  }
  return scope->keep(value);
}

extern "C" int gone_v8_value_kind(gone_v8_scope* scope, uintptr_t id) {
  auto value = scope->get(id);
  if (value.IsEmpty()) return -1;
  if (value->IsUndefined()) return 0;
  if (value->IsNull()) return 1;
  if (value->IsBoolean()) return 2;
  if (value->IsNumber()) return 3;
  if (value->IsString()) return 4;
  if (value->IsFunction()) return 6;
  if (value->IsBigInt()) return 7;
  if (value->IsSymbol()) return 8;
  return 5;
}

extern "C" int gone_v8_value_number(gone_v8_scope* scope,
    uintptr_t id, double* out) {
  auto value = scope->get(id);
  if (value.IsEmpty() || !value->IsNumber()) return 0;
  *out = value.As<v8::Number>()->Value();
  return 1;
}

extern "C" int gone_v8_value_bool(gone_v8_scope* scope, uintptr_t id) {
  auto value = scope->get(id);
  if (value.IsEmpty() || !value->IsBoolean()) return -1;
  return value.As<v8::Boolean>()->Value() ? 1 : 0;
}

extern "C" void gone_v8_value_string(gone_v8_scope* scope,
    uintptr_t id, gone_v8_result* out) {
  auto value = scope->get(id);
  if (value.IsEmpty()) {
    *out = make_result("Invalid value handle", true);
    return;
  }
  v8::TryCatch catcher(scope->vm->isolate);
  v8::Local<v8::String> text;
  if (!value->ToString(scope->context).ToLocal(&text)) {
    *out = exception_result(scope->vm->isolate, scope->context, catcher);
    return;
  }
  *out = make_result(utf8(scope->vm->isolate, text), false);
}

extern "C" uintptr_t gone_v8_value_new(gone_v8_scope* scope, int kind,
    uint64_t bits, const char* text, size_t length, gone_v8_result* error) {
  *error = {nullptr, 0, 0};
  auto isolate = scope->vm->isolate;
  switch (kind) {
    case 0: return scope->keep(v8::Undefined(isolate));
    case 1: return scope->keep(v8::Null(isolate));
    case 2: return scope->keep(v8::Boolean::New(isolate, bits != 0));
    case 3: {
      double number;
      static_assert(sizeof(number) == sizeof(bits), "double ABI mismatch");
      std::memcpy(&number, &bits, sizeof(number));
      return scope->keep(v8::Number::New(isolate, number));
    }
    case 4: {
      if (length > INT_MAX) break;
      v8::Local<v8::String> value;
      if (v8::String::NewFromUtf8(isolate, text, v8::NewStringType::kNormal,
                                static_cast<int>(length)).ToLocal(&value)) {
        return scope->keep(value);
      }
      break;
    }
    default: break;
  }
  *error = make_result("Cannot create value (invalid kind or oversized string)", true);
  return 0;
}

extern "C" uintptr_t gone_v8_value_call(gone_v8_scope* scope,
    uintptr_t function, const uintptr_t* args, size_t count,
    gone_v8_result* error) {
  *error = {nullptr, 0, 0};
  auto fn = scope->get(function);
  if (fn.IsEmpty() || !fn->IsFunction() || count > INT_MAX) {
    *error = make_result("Not a function or too many arguments", true);
    return 0;
  }
  v8::Local<v8::Value> small[16];
  std::vector<v8::Local<v8::Value>> large;
  v8::Local<v8::Value>* argv = small;
  if (count > 16) {
    large.resize(count);
    argv = large.data();
  }
  for (size_t i = 0; i < count; ++i) {
    auto value = scope->get(args[i]);
    if (value.IsEmpty()) {
      *error = make_result("Invalid argument handle", true);
      return 0;
    }
    argv[i] = value;
  }
  auto isolate = scope->vm->isolate;
  v8::TryCatch catcher(isolate);
  v8::Local<v8::Value> value;
  if (!fn.As<v8::Function>()->Call(scope->context, v8::Undefined(isolate),
                                  static_cast<int>(count), argv).ToLocal(&value)) {
    *error = exception_result(isolate, scope->context, catcher);
    return 0;
  }
  return scope->keep(value);
}
#endif
