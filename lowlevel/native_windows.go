//go:build windows && amd64

package lowlevel

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

// The DLL owns all C++ objects and allocations. Only the C ABI crosses into Go.
// Keep it loaded for the process lifetime, like the global V8 platform.
var windowsBridge struct {
	sync.Once
	dll                             *syscall.DLL
	newVM, eval, freeResult, freeVM *syscall.Proc
	err                             error
	low                             [len(windowsLowProcNames)]*syscall.Proc
}

type nativeVM struct{ handle uintptr }

// Layout of gone_v8_result for Windows x64 (size_t is 64 bits).
type windowsResult struct {
	data    unsafe.Pointer
	length  uintptr
	isError int32
}

func bridgeCandidates() []string {
	if path := os.Getenv("GONE_V8_DLL"); path != "" {
		return []string{path}
	}
	var paths []string
	if executable, err := os.Executable(); err == nil {
		paths = append(paths, filepath.Join(filepath.Dir(executable), "gone_v8.dll"))
	}
	// Development fallback for go run/go test. Deploy the DLL next to the exe
	// or set GONE_V8_DLL; builds using -trimpath cannot rely on this fallback.
	if _, file, _, ok := runtime.Caller(0); ok && filepath.IsAbs(file) {
		paths = append(paths, filepath.Join(filepath.Dir(file), "..", "deps", "windows_amd64", "gone_v8.dll"))
	}
	return paths
}

func loadWindowsBridge() {
	var failures []string
	for _, candidate := range bridgeCandidates() {
		path, err := filepath.Abs(candidate)
		if err != nil {
			failures = append(failures, err.Error())
			continue
		}
		dll, err := syscall.LoadDLL(path)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		names := append([]string{"gone_v8_new", "gone_v8_eval_into", "gone_v8_result_free", "gone_v8_free"}, windowsLowProcNames[:]...)
		procs := make([]*syscall.Proc, len(names))
		for i, name := range names {
			procs[i], err = dll.FindProc(name)
			if err != nil {
				break
			}
		}
		if err != nil {
			dll.Release()
			failures = append(failures, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		windowsBridge.dll = dll
		windowsBridge.newVM, windowsBridge.eval = procs[0], procs[1]
		windowsBridge.freeResult, windowsBridge.freeVM = procs[2], procs[3]
		copy(windowsBridge.low[:], procs[4:])
		return
	}
	windowsBridge.err = fmt.Errorf("gone_v8: cannot load Windows bridge; run tools/build-windows.ps1 and deploy gone_v8.dll next to the executable or set GONE_V8_DLL: %s", strings.Join(failures, "; "))
}

func newNativeVM() (*nativeVM, error) {
	windowsBridge.Do(loadWindowsBridge)
	if windowsBridge.err != nil {
		return nil, windowsBridge.err
	}
	handle, _, _ := windowsBridge.newVM.Call()
	if handle == 0 {
		return nil, errors.New("gone_v8: failed to create V8 isolate")
	}
	return &nativeVM{handle: handle}, nil
}

func (vm *nativeVM) eval(source string) result {
	// The bridge consumes the source synchronously and does not retain it.
	var native windowsResult
	syscall.SyscallN(windowsBridge.eval.Addr(), vm.handle, uintptr(unsafe.Pointer(nativeTextPointer(source))), uintptr(len(source)), uintptr(unsafe.Pointer(&native)))
	runtime.KeepAlive(source)
	if native.data == nil {
		return result{err: errors.New("gone_v8: allocation failed")}
	}
	defer syscall.SyscallN(windowsBridge.freeResult.Addr(), uintptr(unsafe.Pointer(&native)))
	text := string(unsafe.Slice((*byte)(native.data), int(native.length)))
	if native.isError != 0 {
		return result{err: errors.New(text)}
	}
	return result{text: text}
}

func (vm *nativeVM) close() { windowsBridge.freeVM.Call(vm.handle) }
