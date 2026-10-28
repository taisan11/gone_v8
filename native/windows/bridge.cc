// Build this translation unit with clang-cl and the matching Chromium libc++.
#include "../../lowlevel/bridge.cc"

static_assert(sizeof(void*) == 8, "The prebuilt V8 library requires Windows x64");
static_assert(sizeof(gone_v8_result) == 24, "Unexpected result ABI");
static_assert(offsetof(gone_v8_result, length) == 8, "Unexpected length offset");
static_assert(offsetof(gone_v8_result, is_error) == 16, "Unexpected error offset");

// Windows x64 returns large structs through a hidden argument. Use an explicit
// out parameter so Go's syscall API does not need C++ struct-return semantics.
extern "C" void gone_v8_eval_into(gone_v8_vm* vm, const char* source,
                                  size_t length, gone_v8_result* out) {
  *out = gone_v8_eval(vm, source, length);
}
