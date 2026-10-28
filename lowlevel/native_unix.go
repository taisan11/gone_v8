//go:build cgo && (linux || darwin)

package lowlevel

/*
#cgo CXXFLAGS: -std=c++20 -fno-rtti -Wall -Wextra -DV8_COMPRESS_POINTERS -DV8_31BIT_SMIS_ON_64BIT_ARCH -I${SRCDIR}/../deps/include
#cgo linux CXXFLAGS: -fPIC
#cgo darwin CXXFLAGS: -stdlib=libc++
#cgo linux,amd64 LDFLAGS: -L${SRCDIR}/../deps/linux_amd64 -lv8 -pthread -ldl
#cgo linux,arm64 LDFLAGS: -L${SRCDIR}/../deps/linux_arm64 -lv8 -pthread -ldl
#cgo darwin,amd64 LDFLAGS: -L${SRCDIR}/../deps/darwin_amd64 -lv8 -pthread
#cgo darwin,arm64 LDFLAGS: -L${SRCDIR}/../deps/darwin_arm64 -lv8 -pthread
#include <stdlib.h>
#include "bridge.h"
*/
import "C"

import (
	"errors"
	"runtime"
	"unsafe"
)

type nativeVM struct{ vm *C.gone_v8_vm }

func newNativeVM() (*nativeVM, error) {
	vm := C.gone_v8_new()
	if vm == nil {
		return nil, errors.New("gone_v8: failed to create V8 isolate")
	}
	return &nativeVM{vm: vm}, nil
}

func (vm *nativeVM) eval(source string) result {
	// gone_v8_eval consumes the Go string synchronously and never retains it.
	native := C.gone_v8_eval(vm.vm, (*C.char)(unsafe.Pointer(nativeTextPointer(source))), C.size_t(len(source)))
	runtime.KeepAlive(source)
	if native.data == nil {
		return result{err: errors.New("gone_v8: allocation failed")}
	}
	defer C.gone_v8_result_free(&native)
	text := C.GoStringN(native.data, C.int(native.length))
	if native.is_error != 0 {
		return result{err: errors.New(text)}
	}
	return result{text: text}
}

func (vm *nativeVM) close() { C.gone_v8_free(vm.vm) }
