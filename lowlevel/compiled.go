//go:build (cgo && (linux || darwin)) || (windows && amd64)

package lowlevel

import "errors"

// CompiledScript is the goroutine-safe convenience wrapper around a Script.
// Every Run is dispatched to its Engine and returns JS ToString output.
// For typed results or no per-run dispatch, use Script.Run inside Engine.Do
// or WithIsolate instead. Do not copy CompiledScript.
type CompiledScript struct {
	engine *Engine
	script *Script // Access only on engine's thread, through Do.
}

// Compile compiles source once in this engine's persistent context without
// executing it. The resulting script can be run repeatedly until closed.
func (e *Engine) Compile(source string) (*CompiledScript, error) {
	compiled := &CompiledScript{engine: e}
	err := e.Do(func(i *Isolate) error {
		var err error
		compiled.script, err = i.Compile(source)
		return err
	})
	if err != nil {
		return nil, err
	}
	return compiled, nil
}

func (compiled *CompiledScript) Run() (string, error) {
	if compiled == nil || compiled.engine == nil {
		return "", ErrClosed
	}
	var text string
	err := compiled.engine.Do(func(i *Isolate) error {
		return i.WithScope(func(scope *Scope) error {
			value, err := compiled.script.Run(scope)
			if err != nil {
				return err
			}
			text, err = value.ToString()
			return err
		})
	})
	return text, err
}

// Close releases the native compiled script. Idempotent, including after its
// Engine has closed (engine teardown already frees its scripts).
func (compiled *CompiledScript) Close() error {
	if compiled == nil || compiled.engine == nil {
		return nil
	}
	err := compiled.engine.Do(func(*Isolate) error { return compiled.script.Close() })
	if errors.Is(err, ErrClosed) {
		return nil
	}
	return err
}
