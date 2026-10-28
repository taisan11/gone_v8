//go:build (cgo && (linux || darwin)) || (windows && amd64)

package lowlevel

import (
	"runtime"
	"testing"
)

const benchmarkSetup = `globalThis.benchCounter = 0;
globalThis.benchIncrement = function() { return ++benchCounter; };`

var benchmarkSources = []struct{ name, source string }{
	{"Empty", ""},
	{"Constant", "1 + 2 * 3"},
	{"Function", "benchIncrement()"},
}

// BenchmarkEval includes dispatch to the locked OS thread, native calls,
// Script::Compile/Run, ToString and result copying. New/Close are not timed.
// Repeated sources use a warm isolate; this is not cold/unique-source parsing.
func BenchmarkEval(b *testing.B) {
	for _, tc := range benchmarkSources {
		b.Run(tc.name, func(b *testing.B) {
			e, err := New()
			if err != nil {
				b.Fatal(err)
			}
			defer e.Close()
			if _, err := e.Eval(benchmarkSetup); err != nil {
				b.Fatal(err)
			}
			for i := 0; i < 1000; i++ {
				if _, err := e.Eval(tc.source); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := e.Eval(tc.source); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
		})
	}
}

// The same native work without Engine's mutex/channel/thread dispatch.
// This is an internal baseline, not a safe public API replacement.
func BenchmarkNativeEval(b *testing.B) {
	for _, tc := range benchmarkSources {
		b.Run(tc.name, func(b *testing.B) {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			vm, err := newNativeVM()
			if err != nil {
				b.Fatal(err)
			}
			defer vm.close()
			if r := vm.eval(benchmarkSetup); r.err != nil {
				b.Fatal(r.err)
			}
			for i := 0; i < 1000; i++ {
				if r := vm.eval(tc.source); r.err != nil {
					b.Fatal(r.err)
				}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if r := vm.eval(tc.source); r.err != nil {
					b.Fatal(r.err)
				}
			}
			b.StopTimer()
		})
	}
}

// Exercise the real Engine.Eval mutex/channel path, but substitute a worker
// that returns a constant without calling V8. Isolates dispatch cost only.
func BenchmarkDispatchOnly(b *testing.B) {
	e := &Engine{jobs: make(chan evaluation), done: make(chan struct{})}
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(e.done)
		for task := range e.jobs {
			task.answer <- result{text: "7"}
		}
	}()
	defer e.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := e.Eval("1 + 2 * 3"); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
}
