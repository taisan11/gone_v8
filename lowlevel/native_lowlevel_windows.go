//go:build windows && amd64

package lowlevel

import (
	"errors"
	"runtime"
	"syscall"
	"unsafe"
)

type nativeScope struct{ handle uintptr }
type nativeScript struct{ handle uintptr }

const (
	lowThreadID = iota
	lowScopeOpen
	lowScopeClose
	lowCompile
	lowScriptFree
	lowScriptRun
	lowScopeEval
	lowValueKind
	lowValueNumber
	lowValueBool
	lowValueString
	lowValueNew
	lowValueCall
	lowScopeIsCurrent
	lowScopeCheck
)

var windowsLowProcNames = [...]string{
	"gone_v8_thread_id", "gone_v8_scope_open", "gone_v8_scope_close",
	"gone_v8_compile", "gone_v8_script_free", "gone_v8_script_run",
	"gone_v8_scope_eval", "gone_v8_value_kind", "gone_v8_value_number",
	"gone_v8_value_bool", "gone_v8_value_string", "gone_v8_value_new",
	"gone_v8_value_call", "gone_v8_scope_is_current", "gone_v8_scope_check",
}

func nativeThreadID() uintptr {
	id, _, _ := syscall.SyscallN(windowsBridge.low[lowThreadID].Addr())
	return id
}

func (vm *nativeVM) openScope() (nativeScope, error) {
	h, _, _ := syscall.SyscallN(windowsBridge.low[lowScopeOpen].Addr(), vm.handle)
	if h == 0 {
		return nativeScope{}, errNativeAllocation
	}
	return nativeScope{h}, nil
}

func (s nativeScope) checkThreadCurrent(thread uintptr) int32 {
	status, _, _ := syscall.SyscallN(windowsBridge.low[lowScopeCheck].Addr(), s.handle, thread)
	return int32(status)
}

func (s nativeScope) close()  { syscall.SyscallN(windowsBridge.low[lowScopeClose].Addr(), s.handle) }
func (s nativeScript) close() { syscall.SyscallN(windowsBridge.low[lowScriptFree].Addr(), s.handle) }

func windowsLowError(out *windowsResult) error {
	if out.isError == 0 {
		return nil
	}
	if out.data == nil {
		return errNativeAllocation
	}
	defer func() { syscall.SyscallN(windowsBridge.freeResult.Addr(), uintptr(unsafe.Pointer(out))) }()
	return errors.New(string(unsafe.Slice((*byte)(out.data), int(out.length))))
}

func windowsLowHandle(h uintptr, out *windowsResult) (uintptr, error) {
	if err := windowsLowError(out); err != nil {
		return 0, err
	}
	if h == 0 {
		return 0, errNativeAllocation
	}
	return h, nil
}

func (s nativeScope) compile(source string) (nativeScript, error) {
	var out windowsResult
	h, _, _ := syscall.SyscallN(windowsBridge.low[lowCompile].Addr(), s.handle,
		uintptr(unsafe.Pointer(nativeTextPointer(source))), uintptr(len(source)), uintptr(unsafe.Pointer(&out)))
	runtime.KeepAlive(source)
	h, err := windowsLowHandle(h, &out)
	return nativeScript{h}, err
}

func (s nativeScope) evalValue(source string) (uintptr, error) {
	var out windowsResult
	h, _, _ := syscall.SyscallN(windowsBridge.low[lowScopeEval].Addr(), s.handle,
		uintptr(unsafe.Pointer(nativeTextPointer(source))), uintptr(len(source)), uintptr(unsafe.Pointer(&out)))
	runtime.KeepAlive(source)
	return windowsLowHandle(h, &out)
}

func (s nativeScope) run(script nativeScript) (uintptr, error) {
	var out windowsResult
	h, _, _ := syscall.SyscallN(windowsBridge.low[lowScriptRun].Addr(), s.handle, script.handle, uintptr(unsafe.Pointer(&out)))
	return windowsLowHandle(h, &out)
}

func (s nativeScope) kind(id uintptr) int32 {
	k, _, _ := syscall.SyscallN(windowsBridge.low[lowValueKind].Addr(), s.handle, id)
	return int32(k)
}

func (s nativeScope) number(id uintptr) (float64, bool) {
	var value float64
	ok, _, _ := syscall.SyscallN(windowsBridge.low[lowValueNumber].Addr(), s.handle, id, uintptr(unsafe.Pointer(&value)))
	return value, ok != 0
}

func (s nativeScope) boolean(id uintptr) int32 {
	value, _, _ := syscall.SyscallN(windowsBridge.low[lowValueBool].Addr(), s.handle, id)
	return int32(value)
}

func (s nativeScope) valueString(id uintptr) (string, error) {
	var out windowsResult
	syscall.SyscallN(windowsBridge.low[lowValueString].Addr(), s.handle, id, uintptr(unsafe.Pointer(&out)))
	if err := windowsLowError(&out); err != nil {
		return "", err
	}
	if out.data == nil {
		return "", errNativeAllocation
	}
	defer func() { syscall.SyscallN(windowsBridge.freeResult.Addr(), uintptr(unsafe.Pointer(&out))) }()
	return string(unsafe.Slice((*byte)(out.data), int(out.length))), nil
}

func (s nativeScope) newValue(kind ValueKind, bits uint64, text string) (uintptr, error) {
	var out windowsResult
	h, _, _ := syscall.SyscallN(windowsBridge.low[lowValueNew].Addr(), s.handle, uintptr(kind), uintptr(bits),
		uintptr(unsafe.Pointer(nativeTextPointer(text))), uintptr(len(text)), uintptr(unsafe.Pointer(&out)))
	runtime.KeepAlive(text)
	return windowsLowHandle(h, &out)
}

func (s nativeScope) call(id uintptr, args []uintptr) (uintptr, error) {
	var out windowsResult
	h, _, _ := syscall.SyscallN(windowsBridge.low[lowValueCall].Addr(), s.handle, id,
		uintptr(unsafe.Pointer(unsafe.SliceData(args))), uintptr(len(args)), uintptr(unsafe.Pointer(&out)))
	runtime.KeepAlive(args)
	return windowsLowHandle(h, &out)
}
