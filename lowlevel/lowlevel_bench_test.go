//go:build (cgo && (linux || darwin)) || (windows && amd64)

package lowlevel

import "testing"

// Bound native local-handle retention. Every batch closes its HandleScope.
const benchmarkScopeBatch = 256

func BenchmarkCompiledDirect(b *testing.B) {
	for _, readNumber := range []bool{false, true} {
		name := "RunOnly"
		if readNumber {
			name = "RunFloat64"
		}
		b.Run(name, func(b *testing.B) {
			err := WithIsolate(func(i *Isolate) error {
				script, err := i.Compile("1 + 2 * 3")
				if err != nil {
					return err
				}
				defer script.Close()
				if err := i.WithScope(func(s *Scope) error {
					for n := 0; n < 1000; n++ {
						if _, err := script.Run(s); err != nil {
							return err
						}
					}
					return nil
				}); err != nil {
					return err
				}
				b.ReportAllocs()
				b.ResetTimer()
				for base := 0; base < b.N; {
					count := min(benchmarkScopeBatch, b.N-base)
					err := i.WithScope(func(s *Scope) error {
						for n := 0; n < count; n++ {
							v, err := script.Run(s)
							if err != nil {
								return err
							}
							if readNumber {
								if _, err := v.Float64(); err != nil {
									return err
								}
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

func BenchmarkCompiledScopePerRun(b *testing.B) {
	err := WithIsolate(func(i *Isolate) error {
		script, err := i.Compile("1 + 2 * 3")
		if err != nil {
			return err
		}
		defer script.Close()
		b.ReportAllocs()
		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			if err := i.WithScope(func(s *Scope) error {
				v, err := script.Run(s)
				if err != nil {
					return err
				}
				_, err = v.Float64()
				return err
			}); err != nil {
				return err
			}
		}
		b.StopTimer()
		return nil
	})
	if err != nil {
		b.Fatal(err)
	}
}

func BenchmarkEngineCompiledRun(b *testing.B) {
	e, err := New()
	if err != nil {
		b.Fatal(err)
	}
	defer e.Close()
	script, err := e.Compile("1 + 2 * 3")
	if err != nil {
		b.Fatal(err)
	}
	defer script.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		if _, err := script.Run(); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
}

// One Engine.Do round-trip per batch, not per Script.Run.
func BenchmarkEngineDoCompiledBatch(b *testing.B) {
	e, err := New()
	if err != nil {
		b.Fatal(err)
	}
	defer e.Close()
	var script *Script
	if err := e.Do(func(i *Isolate) error {
		var err error
		script, err = i.Compile("1 + 2 * 3")
		return err
	}); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for base := 0; base < b.N; {
		count := min(benchmarkScopeBatch, b.N-base)
		if err := e.Do(func(i *Isolate) error {
			return i.WithScope(func(s *Scope) error {
				for n := 0; n < count; n++ {
					v, err := script.Run(s)
					if err != nil {
						return err
					}
					if _, err := v.Float64(); err != nil {
						return err
					}
				}
				return nil
			})
		}); err != nil {
			b.Fatal(err)
		}
		base += count
	}
	b.StopTimer()
}
