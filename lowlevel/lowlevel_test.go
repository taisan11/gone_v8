//go:build (cgo && (linux || darwin)) || (windows && amd64)

package lowlevel

import (
	"errors"
	"fmt"
	"math"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func TestLowLevelCompiledScript(t *testing.T) {
	err := WithIsolate(func(i *Isolate) error {
		err := i.WithScope(func(s *Scope) error {
			_, err := s.Eval("globalThis.counter = 0")
			return err
		})
		if err != nil {
			return err
		}
		script, err := i.Compile("++counter")
		if err != nil {
			return err
		}
		defer script.Close()
		for want := 1; want <= 100; want++ {
			if err := i.WithScope(func(s *Scope) error {
				v, err := script.Run(s)
				if err != nil {
					return err
				}
				n, err := v.Float64()
				if err != nil || n != float64(want) {
					return fmt.Errorf("run = %v, %v; want %d", n, err, want)
				}
				return nil
			}); err != nil {
				return err
			}
		}
		if err := script.Close(); err != nil {
			return err
		}
		if err := script.Close(); err != nil {
			return err
		}
		return i.WithScope(func(s *Scope) error {
			_, err := script.Run(s)
			if !errors.Is(err, ErrClosed) {
				return fmt.Errorf("closed script: %v", err)
			}
			return nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestLowLevelValueKinds(t *testing.T) {
	err := WithIsolate(func(i *Isolate) error {
		return i.WithScope(func(s *Scope) error {
			cases := []struct {
				source string
				kind   ValueKind
			}{
				{"undefined", KindUndefined}, {"null", KindNull}, {"true", KindBoolean},
				{"42", KindNumber}, {`"text"`, KindString}, {"({})", KindObject},
				{"(() => 1)", KindFunction}, {"123n", KindBigInt}, {"Symbol('s')", KindSymbol},
			}
			for _, tc := range cases {
				v, err := s.Eval(tc.source)
				if err != nil {
					return err
				}
				kind, err := v.Kind()
				if err != nil || kind != tc.kind {
					return fmt.Errorf("kind(%s) = %v, %v", tc.source, kind, err)
				}
			}
			return nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestLowLevelConstructorsAndCall(t *testing.T) {
	err := WithIsolate(func(i *Isolate) error {
		return i.WithScope(func(s *Scope) error {
			for _, number := range []float64{0, math.Copysign(0, -1), 42.5, math.Inf(1), math.Inf(-1), math.NaN()} {
				v, err := s.Number(number)
				if err != nil {
					return err
				}
				got, err := v.Float64()
				if err != nil || (!math.IsNaN(number) && math.Float64bits(got) != math.Float64bits(number)) || (math.IsNaN(number) && !math.IsNaN(got)) {
					return fmt.Errorf("number round trip %v = %v, %v", number, got, err)
				}
			}
			for _, boolean := range []bool{true, false} {
				v, err := s.Boolean(boolean)
				if err != nil {
					return err
				}
				got, err := v.Bool()
				if err != nil || got != boolean {
					return fmt.Errorf("boolean = %v, %v", got, err)
				}
				if _, err := v.Float64(); !errors.Is(err, ErrType) {
					return fmt.Errorf("boolean coerced to number: %v", err)
				}
			}
			for _, text := range []string{"", "こんにちは🌍", "a\x00b"} {
				v, err := s.String(text)
				if err != nil {
					return err
				}
				got, err := v.ToString()
				if err != nil || got != text {
					return fmt.Errorf("string = %q, %v; want %q", got, err, text)
				}
				if _, err := v.Bool(); !errors.Is(err, ErrType) {
					return fmt.Errorf("string coerced to bool: %v", err)
				}
			}
			undefined, err := s.Undefined()
			if err != nil {
				return err
			}
			if kind, _ := undefined.Kind(); kind != KindUndefined {
				return fmt.Errorf("undefined kind: %v", kind)
			}
			null, err := s.Null()
			if err != nil {
				return err
			}
			if kind, _ := null.Kind(); kind != KindNull {
				return fmt.Errorf("null kind: %v", kind)
			}
			fn, err := s.Eval("((a, b) => a + b)")
			if err != nil {
				return err
			}
			a, err := s.Number(20)
			if err != nil {
				return err
			}
			b, err := s.Number(22)
			if err != nil {
				return err
			}
			v, err := fn.Call(a, b)
			if err != nil {
				return err
			}
			if n, err := v.Float64(); err != nil || n != 42 {
				return fmt.Errorf("call = %v, %v", n, err)
			}
			sum, err := s.Eval("((...args) => args.reduce((a,b) => a+b, 0))")
			if err != nil {
				return err
			}
			for _, count := range []int{8, 9, 16, 17} {
				many := make([]Value, count)
				for index := range many {
					many[index] = a
				}
				large, err := sum.Call(many...)
				if err != nil {
					return err
				}
				if n, err := large.Float64(); err != nil || n != float64(count*20) {
					return fmt.Errorf("call with %d arguments = %v, %v", count, n, err)
				}
			}
			strict, err := s.Eval("(function() { 'use strict'; return this === undefined; })")
			if err != nil {
				return err
			}
			this, err := strict.Call()
			if err != nil {
				return err
			}
			if got, err := this.Bool(); err != nil || !got {
				return fmt.Errorf("receiver is not undefined: %v, %v", got, err)
			}
			if _, err := a.Call(); err == nil {
				return errors.New("non-function call succeeded")
			}
			if _, err := fn.Call(Value{}); !errors.Is(err, ErrInvalidValue) {
				return fmt.Errorf("invalid argument: %v", err)
			}
			return nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestLowLevelExceptions(t *testing.T) {
	err := WithIsolate(func(i *Isolate) error {
		if _, err := i.Compile("let ="); err == nil {
			return errors.New("invalid source compiled")
		}
		script, err := i.Compile("throw new Error('run failed')")
		if err != nil {
			return err
		}
		return i.WithScope(func(s *Scope) error {
			if _, err := script.Run(s); err == nil || !strings.Contains(err.Error(), "run failed") {
				return fmt.Errorf("run exception: %v", err)
			}
			if _, err := s.Eval("throw new Error('eval failed')"); err == nil || !strings.Contains(err.Error(), "eval failed") {
				return fmt.Errorf("eval exception: %v", err)
			}
			v, err := s.Eval("({toString() { throw new Error('conversion failed') }})")
			if err != nil {
				return err
			}
			if _, err := v.ToString(); err == nil || !strings.Contains(err.Error(), "conversion failed") {
				return fmt.Errorf("conversion exception: %v", err)
			}
			fn, err := s.Eval("(() => { throw new Error('call failed') })")
			if err != nil {
				return err
			}
			if _, err := fn.Call(); err == nil || !strings.Contains(err.Error(), "call failed") {
				return fmt.Errorf("call exception: %v", err)
			}
			_, err = s.Eval("42") // Exceptions must not poison subsequent calls.
			return err
		})
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestLowLevelHandlesSurviveGC(t *testing.T) {
	err := WithIsolate(func(i *Isolate) error {
		return i.WithScope(func(s *Scope) error {
			fn, err := s.Eval("(() => { const text = 'retained'; return () => text; })()")
			if err != nil {
				return err
			}
			runtime.GC()
			if _, err := s.Eval("(() => { for (let n = 0; n < 2000; ++n) globalThis.garbage = new Array(4096).fill(n); })()"); err != nil {
				return err
			}
			v, err := fn.Call()
			if err != nil {
				return err
			}
			got, err := v.ToString()
			if err != nil || got != "retained" {
				return fmt.Errorf("value after GC = %q, %v", got, err)
			}
			return nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestLowLevelLifetimesAndScopes(t *testing.T) {
	err := WithIsolate(func(i *Isolate) error {
		var expired Value
		var closedScope *Scope
		err := i.WithScope(func(s *Scope) error {
			closedScope = s
			fn, err := s.Eval("x => x")
			if err != nil {
				return err
			}
			expired, err = s.Eval("42")
			if err != nil {
				return err
			}
			if err := i.WithScope(func(child *Scope) error {
				if _, err := expired.Kind(); !errors.Is(err, ErrScopeNotCurrent) {
					return fmt.Errorf("parent used in child: %v", err)
				}
				if _, err := fn.Call(expired); !errors.Is(err, ErrScopeNotCurrent) {
					return fmt.Errorf("parent call in child: %v", err)
				}
				_, err := child.Eval("1")
				return err
			}); err != nil {
				return err
			}
			_, err = expired.Float64() // Parent becomes current again.
			return err
		})
		if err != nil {
			return err
		}
		if _, err := expired.Kind(); !errors.Is(err, ErrScopeClosed) {
			return fmt.Errorf("expired value: %v", err)
		}
		if _, err := closedScope.Eval("1"); !errors.Is(err, ErrScopeClosed) {
			return fmt.Errorf("closed scope: %v", err)
		}
		if _, err := (Value{}).ToString(); !errors.Is(err, ErrInvalidValue) {
			return fmt.Errorf("zero value: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestLowLevelScopeCheckAfterIsolateClose(t *testing.T) {
	var value Value
	if err := WithIsolate(func(i *Isolate) error {
		return i.WithScope(func(s *Scope) error {
			var err error
			value, err = s.Eval("42")
			return err
		})
	}); err != nil {
		t.Fatal(err)
	}
	// Use a locked OS thread so the thread token cannot change across checks.
	// Either a different thread or the closed isolate must be reported; the
	// already-freed native scope must never be dereferenced.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if _, err := value.Kind(); !errors.Is(err, ErrClosed) && !errors.Is(err, ErrWrongThread) {
		t.Fatalf("value after isolate close: %v", err)
	}
}

func TestLowLevelWrongThreadAndIsolate(t *testing.T) {
	err := WithIsolate(func(i *Isolate) error {
		return i.WithScope(func(s *Scope) error {
			value, err := s.Eval("42")
			if err != nil {
				return err
			}
			fn, err := s.Eval("x => x")
			if err != nil {
				return err
			}
			answer := make(chan error, 1)
			go func() { _, err := value.Kind(); answer <- err }()
			if err := <-answer; !errors.Is(err, ErrWrongThread) {
				return fmt.Errorf("wrong thread: %v", err)
			}
			go func() { _, err := fn.Call(value); answer <- err }()
			if err := <-answer; !errors.Is(err, ErrWrongThread) {
				return fmt.Errorf("call on wrong thread: %v", err)
			}
			script, err := s.Compile("42")
			if err != nil {
				return err
			}
			return WithIsolate(func(other *Isolate) error {
				return other.WithScope(func(otherScope *Scope) error {
					if _, err := value.Float64(); !errors.Is(err, ErrScopeNotCurrent) {
						return fmt.Errorf("outer isolate's scope used while another is entered: %v", err)
					}
					if _, err := script.Run(otherScope); !errors.Is(err, ErrWrongIsolate) {
						return fmt.Errorf("wrong isolate: %v", err)
					}
					fn, err := otherScope.Eval("(x => x)")
					if err != nil {
						return err
					}
					if _, err := fn.Call(value); !errors.Is(err, ErrWrongScope) {
						return fmt.Errorf("wrong argument scope: %v", err)
					}
					return nil
				})
			})
		})
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestEngineDoAndPanicCleanup(t *testing.T) {
	e, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	sentinel := errors.New("callback error")
	if err := e.Do(func(*Isolate) error { return sentinel }); err != sentinel {
		t.Fatalf("callback error: %v", err)
	}
	marker := &struct{}{}
	func() {
		defer func() {
			if got := recover(); got != marker {
				t.Errorf("panic = %v", got)
			}
		}()
		e.Do(func(i *Isolate) error {
			return i.WithScope(func(s *Scope) error {
				if _, err := s.Eval("globalThis.fromDo = 42"); err != nil {
					return err
				}
				panic(marker)
			})
		})
	}()
	if got, err := e.Eval("fromDo"); err != nil || got != "42" {
		t.Fatalf("engine after callback panic: %q, %v", got, err)
	}
	if err := e.Do(func(i *Isolate) error { return i.WithScope(func(s *Scope) error { _, err := s.Eval("1"); return err }) }); err != nil {
		t.Fatal(err)
	}
}

func TestEngineReentrantCalls(t *testing.T) {
	e, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if err := e.Do(func(i *Isolate) error {
		if _, err := e.Eval("1"); !errors.Is(err, ErrReentrant) {
			return fmt.Errorf("reentrant Eval: %v", err)
		}
		if _, err := e.Compile("1"); !errors.Is(err, ErrReentrant) {
			return fmt.Errorf("reentrant Compile: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if got := recover(); got != ErrReentrant {
				t.Errorf("reentrant Close panic: %v", got)
			}
		}()
		e.Do(func(*Isolate) error { e.Close(); return nil })
	}()
	if got, err := e.Eval("42"); err != nil || got != "42" {
		t.Fatalf("engine after reentrant Close: %q, %v", got, err)
	}
}

func TestLowLevelCopiedResources(t *testing.T) {
	err := WithIsolate(func(i *Isolate) error {
		copiedIsolate := *i
		if _, err := copiedIsolate.Compile("1"); !errors.Is(err, ErrCopiedResource) {
			return fmt.Errorf("copied isolate: %v", err)
		}
		script, err := i.Compile("42")
		if err != nil {
			return err
		}
		copiedScript := *script
		if err := copiedScript.Close(); !errors.Is(err, ErrCopiedResource) {
			return fmt.Errorf("copied script Close: %v", err)
		}
		return i.WithScope(func(s *Scope) error {
			if _, err := copiedScript.Run(s); !errors.Is(err, ErrCopiedResource) {
				return fmt.Errorf("copied script Run: %v", err)
			}
			_, err := script.Run(s)
			return err
		})
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestLowLevelZeroValues(t *testing.T) {
	var i Isolate
	if _, err := i.Compile("1"); !errors.Is(err, ErrClosed) {
		t.Fatalf("zero isolate: %v", err)
	}
	var script Script
	if err := script.Close(); !errors.Is(err, ErrClosed) {
		t.Fatalf("zero script: %v", err)
	}
	if err := WithIsolate(nil); err == nil {
		t.Fatal("nil callback accepted")
	}
	if err := WithIsolate(func(i *Isolate) error {
		if err := i.WithScope(nil); err == nil {
			return errors.New("nil scope callback accepted")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestEngineCallbackGoexitDoesNotHang(t *testing.T) {
	e, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if err := e.Do(func(i *Isolate) error {
		return i.WithScope(func(*Scope) error { runtime.Goexit(); return nil })
	}); !errors.Is(err, ErrClosed) {
		t.Fatalf("Goexit error: %v", err)
	}
	if _, err := e.Eval("1"); !errors.Is(err, ErrClosed) {
		t.Fatalf("worker exited: %v", err)
	}
}

func TestEngineCompiledScriptConcurrent(t *testing.T) {
	e, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if _, err := e.Eval("globalThis.compiledCounter = 0"); err != nil {
		t.Fatal(err)
	}
	script, err := e.Compile("++compiledCounter")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for n := 0; n < 8; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				if _, err := script.Run(); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()
	if got, err := e.Eval("compiledCounter"); err != nil || got != "160" {
		t.Fatalf("counter = %q, %v", got, err)
	}
	if _, err := e.Compile("let ="); err == nil {
		t.Fatal("invalid source compiled")
	}
	if err := script.Close(); err != nil {
		t.Fatal(err)
	}
	if err := script.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := script.Run(); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed compiled script: %v", err)
	}
	orphan, err := e.Compile("42") // Engine.Close must release this one.
	if err != nil {
		t.Fatal(err)
	}
	e.Close()
	if _, err := orphan.Run(); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed engine: %v", err)
	}
	if err := orphan.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestWithIsolatePanicAndAutomaticCleanup(t *testing.T) {
	var script *Script
	func() {
		defer func() {
			if got := recover(); got != "cleanup" {
				t.Errorf("panic = %v", got)
			}
		}()
		WithIsolate(func(i *Isolate) error {
			var err error
			script, err = i.Compile("42")
			if err != nil {
				t.Fatal(err)
			}
			return i.WithScope(func(*Scope) error { panic("cleanup") })
		})
	}()
	if script == nil || !script.closed {
		t.Fatal("persistent script not freed on panic")
	}
	if err := WithIsolate(func(i *Isolate) error {
		return i.WithScope(func(s *Scope) error { _, err := s.Eval("42"); return err })
	}); err != nil {
		t.Fatal(err)
	}
}
