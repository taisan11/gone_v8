//go:build (cgo && (linux || darwin)) || (windows && amd64)

package lowlevel

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestEval(t *testing.T) {
	e, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	got, err := e.Eval("1 + 2 * 3")
	if err != nil || got != "7" {
		t.Fatalf("got (%q, %v), want 7", got, err)
	}
	if _, err := e.Eval("globalThis.x = 41"); err != nil {
		t.Fatal(err)
	}
	got, err = e.Eval("x + 1")
	if err != nil || got != "42" {
		t.Fatalf("persistent context: got %q, %v", got, err)
	}
	_, err = e.Eval("throw new Error('boom')")
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("exception: %v", err)
	}
}

func TestEvalStringsAndErrors(t *testing.T) {
	e, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	for _, tc := range []struct{ source, want string }{
		{"", "undefined"},
		{`'こんにちは🌍'`, "こんにちは🌍"},
		{`'a\u0000b'`, "a\x00b"},
		{`'a` + "\x00" + `b'`, "a\x00b"},
	} {
		got, err := e.Eval(tc.source)
		if err != nil || got != tc.want {
			t.Errorf("Eval(%q) = %q, %v; want %q", tc.source, got, err, tc.want)
		}
	}
	for _, source := range []string{"let =", `({toString() { throw new Error('conversion failed') }})`} {
		if _, err := e.Eval(source); err == nil {
			t.Errorf("Eval(%q) should fail", source)
		}
	}
}

func TestIndependentEnginesAndClose(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if _, err := a.Eval("globalThis.onlyInA = 1"); err != nil {
		t.Fatal(err)
	}
	if got, err := b.Eval("typeof onlyInA"); err != nil || got != "undefined" {
		t.Fatalf("isolates share state: %q, %v", got, err)
	}
	a.Close()
	a.Close()
	if _, err := a.Eval("1"); err == nil {
		t.Fatal("Eval after Close should fail")
	}
	if got, err := b.Eval("6 * 7"); err != nil || got != "42" {
		t.Fatalf("closing one engine broke another: %q, %v", got, err)
	}
}

func TestConcurrentEvalDistinctResults(t *testing.T) {
	e, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			want := fmt.Sprintf("caller-%d", id)
			for n := 0; n < 50; n++ {
				if n%3 == 0 {
					_, err := e.Eval(fmt.Sprintf("throw new Error('%s')", want))
					if err == nil || !strings.Contains(err.Error(), want) {
						t.Errorf("wrong exception for %s: %v", want, err)
						return
					}
				} else {
					got, err := e.Eval(fmt.Sprintf("'%s'", want))
					if err != nil || got != want {
						t.Errorf("Eval for %s: %q, %v", want, got, err)
						return
					}
				}
			}
		}(i)
	}
	wg.Wait()
}

func TestConcurrentEval(t *testing.T) {
	e, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := e.Eval("1 + 1"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
}
