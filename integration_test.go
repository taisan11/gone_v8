//go:build (cgo && (linux || darwin)) || (windows && amd64)

package gone_v8_test

import (
	"testing"

	v8 "github.com/taisan11/gone_v8"
	"github.com/taisan11/gone_v8/lowlevel"
)

func TestEngineLowlevelIntegration(t *testing.T) {
	e, err := v8.New()
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if err := e.Do(func(i *lowlevel.Isolate) error {
		return i.WithScope(func(s *lowlevel.Scope) error {
			v, err := s.Eval("40 + 2")
			if err != nil {
				return err
			}
			n, err := v.Float64()
			if err != nil {
				return err
			}
			if n != 42 {
				t.Errorf("got %v, want 42", n)
			}
			return nil
		})
	}); err != nil {
		t.Fatal(err)
	}
	// Root aliases remain usable with the separate low-level package.
	if err := v8.WithIsolate(func(i *v8.Isolate) error { return nil }); err != nil {
		t.Fatal(err)
	}
}
