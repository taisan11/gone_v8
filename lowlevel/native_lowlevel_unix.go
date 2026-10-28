//go:build cgo && (linux || darwin)

package lowlevel

/*
#include "bridge.h"
*/
import "C"

import (
	"errors"
	"runtime"
	"unsafe"
)

type nativeScope struct{ handle *C.gone_v8_scope }
type nativeScript struct{ handle *C.gone_v8_script }

func nativeThreadID() uintptr { return uintptr(C.gone_v8_thread_id()) }

func (vm *nativeVM) openScope() (nativeScope, error) {
	h := C.gone_v8_scope_open(vm.vm)
	if h == nil {
		return nativeScope{}, errNativeAllocation
	}
	return nativeScope{h}, nil
}

func (s nativeScope) checkThreadCurrent(thread uintptr) int32 {
	return int32(C.gone_v8_scope_check(s.handle, C.uintptr_t(thread)))
}

func (s nativeScope) close()  { C.gone_v8_scope_close(s.handle) }
func (s nativeScript) close() { C.gone_v8_script_free(s.handle) }

func unixLowError(out *C.gone_v8_result) error {
	if out.is_error == 0 {
		return nil
	}
	if out.data == nil {
		return errNativeAllocation
	}
	defer C.gone_v8_result_free(out)
	return errors.New(C.GoStringN(out.data, C.int(out.length)))
}

func unixLowHandle(h C.uintptr_t, out *C.gone_v8_result) (uintptr, error) {
	if err := unixLowError(out); err != nil {
		return 0, err
	}
	if h == 0 {
		return 0, errNativeAllocation
	}
	return uintptr(h), nil
}

func (s nativeScope) compile(source string) (nativeScript, error) {
	var out C.gone_v8_result
	h := C.gone_v8_compile(s.handle, (*C.char)(unsafe.Pointer(nativeTextPointer(source))), C.size_t(len(source)), &out)
	runtime.KeepAlive(source)
	if err := unixLowError(&out); err != nil {
		return nativeScript{}, err
	}
	if h == nil {
		return nativeScript{}, errNativeAllocation
	}
	return nativeScript{h}, nil
}

func (s nativeScope) evalValue(source string) (uintptr, error) {
	var out C.gone_v8_result
	h := C.gone_v8_scope_eval(s.handle, (*C.char)(unsafe.Pointer(nativeTextPointer(source))), C.size_t(len(source)), &out)
	runtime.KeepAlive(source)
	return unixLowHandle(h, &out)
}

func (s nativeScope) run(script nativeScript) (uintptr, error) {
	var out C.gone_v8_result
	h := C.gone_v8_script_run(s.handle, script.handle, &out)
	return unixLowHandle(h, &out)
}

func (s nativeScope) kind(id uintptr) int32 {
	return int32(C.gone_v8_value_kind(s.handle, C.uintptr_t(id)))
}

func (s nativeScope) number(id uintptr) (float64, bool) {
	var value C.double
	ok := C.gone_v8_value_number(s.handle, C.uintptr_t(id), &value)
	return float64(value), ok != 0
}

func (s nativeScope) boolean(id uintptr) int32 {
	return int32(C.gone_v8_value_bool(s.handle, C.uintptr_t(id)))
}

func (s nativeScope) valueString(id uintptr) (string, error) {
	var out C.gone_v8_result
	C.gone_v8_value_string(s.handle, C.uintptr_t(id), &out)
	if err := unixLowError(&out); err != nil {
		return "", err
	}
	if out.data == nil {
		return "", errNativeAllocation
	}
	defer C.gone_v8_result_free(&out)
	return C.GoStringN(out.data, C.int(out.length)), nil
}

func (s nativeScope) newValue(kind ValueKind, bits uint64, text string) (uintptr, error) {
	var out C.gone_v8_result
	h := C.gone_v8_value_new(s.handle, C.int(kind), C.uint64_t(bits),
		(*C.char)(unsafe.Pointer(nativeTextPointer(text))), C.size_t(len(text)), &out)
	runtime.KeepAlive(text)
	return unixLowHandle(h, &out)
}

func (s nativeScope) call(id uintptr, args []uintptr) (uintptr, error) {
	var out C.gone_v8_result
	h := C.gone_v8_value_call(s.handle, C.uintptr_t(id),
		(*C.uintptr_t)(unsafe.Pointer(unsafe.SliceData(args))), C.size_t(len(args)), &out)
	runtime.KeepAlive(args)
	return unixLowHandle(h, &out)
}
