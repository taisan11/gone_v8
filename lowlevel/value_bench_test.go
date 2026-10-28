//go:build (cgo && (linux || darwin)) || (windows && amd64)

package lowlevel

import "testing"

// Scope setup and construction of the input value are amortized over each
// batch. Native allocations are not reflected in Go's allocation statistics.
func BenchmarkValueOperations(b *testing.B) {
	for _, operation := range []string{"Kind", "Float64", "Bool", "Number", "Boolean"} {
		b.Run(operation, func(b *testing.B) {
			err := WithIsolate(func(i *Isolate) error {
				b.ReportAllocs()
				b.ResetTimer()
				for base := 0; base < b.N; {
					count := min(benchmarkScopeBatch, b.N-base)
					err := i.WithScope(func(s *Scope) error {
						number, err := s.Number(42)
						if err != nil {
							return err
						}
						boolean, err := s.Boolean(true)
						if err != nil {
							return err
						}
						for n := 0; n < count; n++ {
							switch operation {
							case "Kind":
								_, err = number.Kind()
							case "Float64":
								_, err = number.Float64()
							case "Bool":
								_, err = boolean.Bool()
							case "Number":
								_, err = s.Number(42)
							case "Boolean":
								_, err = s.Boolean(true)
							}
							if err != nil {
								return err
							}
						}
						return nil
					})
					if err != nil {
						return err
					}
					base += count
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
