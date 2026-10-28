//go:build (cgo && (linux || darwin)) || (windows && amd64)

package lowlevel

import (
	"fmt"
	"testing"
)

func BenchmarkValueCall(b *testing.B) {
	for _, count := range []int{0, 2, 8, 16} {
		b.Run(fmt.Sprintf("Args%d", count), func(b *testing.B) {
			err := WithIsolate(func(i *Isolate) error {
				b.ReportAllocs()
				b.ResetTimer()
				for base := 0; base < b.N; {
					countRuns := min(benchmarkScopeBatch, b.N-base)
					err := i.WithScope(func(s *Scope) error {
						fn, err := s.Eval("((...args) => args.length)")
						if err != nil {
							return err
						}
						arg, err := s.Number(1)
						if err != nil {
							return err
						}
						args := make([]Value, count)
						for j := range args {
							args[j] = arg
						}
						for j := 0; j < countRuns; j++ {
							if _, err := fn.Call(args...); err != nil {
								return err
							}
						}
						return nil
					})
					if err != nil {
						return err
					}
					base += countRuns
				}
				b.StopTimer()
				return nil
			})
			if err != nil {
				b.Fatal(err)
			}
		})
	}
}
