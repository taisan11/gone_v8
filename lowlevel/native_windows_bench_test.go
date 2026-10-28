//go:build windows && amd64

package lowlevel

import (
	"runtime"
	"testing"
	"unsafe"
)

// A near-empty DLL call: free a zero result (free(nullptr)), with no V8 work.
// This is a syscall.Proc.Call baseline, not the cost of a complete Eval.
func BenchmarkWindowsDLLCall(b *testing.B) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	windowsBridge.Do(loadWindowsBridge)
	if windowsBridge.err != nil {
		b.Fatal(windowsBridge.err)
	}
	var native windowsResult
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		windowsBridge.freeResult.Call(uintptr(unsafe.Pointer(&native)))
	}
	b.StopTimer()
	runtime.KeepAlive(&native)
}
