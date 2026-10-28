#ifndef GONE_V8_BRIDGE_H
#define GONE_V8_BRIDGE_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef struct gone_v8_vm gone_v8_vm;

typedef struct {
  char* data;
  size_t length;
  int is_error;
} gone_v8_result;

// Initializes the global V8 platform once, then creates an isolate and context.
// This and all VM operations must run on the SAME locked OS thread.
gone_v8_vm* gone_v8_new(void);
gone_v8_result gone_v8_eval(gone_v8_vm* vm, const char* source, size_t len);
void gone_v8_result_free(gone_v8_result* result);
void gone_v8_free(gone_v8_vm* vm);

// Low-level API. Scopes and all operations are thread-affine. Value IDs are
// local to their scope; compiled scripts are persistent and isolate-bound.
typedef struct gone_v8_scope gone_v8_scope;
typedef struct gone_v8_script gone_v8_script;
uintptr_t gone_v8_thread_id(void);
gone_v8_scope* gone_v8_scope_open(gone_v8_vm* vm);
void gone_v8_scope_close(gone_v8_scope* scope);
int gone_v8_scope_is_current(gone_v8_scope* scope);
// -1: wrong thread, 0: not current, 1: current. Never dereferences scope.
int gone_v8_scope_check(gone_v8_scope* scope, uintptr_t owner_thread);
gone_v8_script* gone_v8_compile(gone_v8_scope* scope, const char* source,
                              size_t length, gone_v8_result* error);
void gone_v8_script_free(gone_v8_script* script);
uintptr_t gone_v8_script_run(gone_v8_scope* scope, gone_v8_script* script,
                             gone_v8_result* error);
uintptr_t gone_v8_scope_eval(gone_v8_scope* scope, const char* source,
                             size_t length, gone_v8_result* error);
// Kind: undefined=0, null=1, boolean=2, number=3, string=4, object=5,
// function=6, bigint=7, symbol=8. -1 means an invalid handle.
int gone_v8_value_kind(gone_v8_scope* scope, uintptr_t value);
int gone_v8_value_number(gone_v8_scope* scope, uintptr_t value, double* out);
int gone_v8_value_bool(gone_v8_scope* scope, uintptr_t value);
void gone_v8_value_string(gone_v8_scope* scope, uintptr_t value,
                         gone_v8_result* out);
uintptr_t gone_v8_value_new(gone_v8_scope* scope, int kind, uint64_t bits,
                            const char* text, size_t length,
                            gone_v8_result* error);
uintptr_t gone_v8_value_call(gone_v8_scope* scope, uintptr_t function,
                             const uintptr_t* args, size_t count,
                             gone_v8_result* error);

#ifdef __cplusplus
}
#endif

#endif
