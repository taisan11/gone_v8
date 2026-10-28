//go:build (cgo && (linux || darwin)) || (windows && amd64)

package lowlevel

import "math"

// ValueKind describes a JavaScript value without coercing it.
type ValueKind int32

const (
	KindUndefined ValueKind = iota
	KindNull
	KindBoolean
	KindNumber
	KindString
	KindObject
	KindFunction
	KindBigInt
	KindSymbol
)

func (kind ValueKind) String() string {
	names := [...]string{"undefined", "null", "boolean", "number", "string", "object", "function", "bigint", "symbol"}
	if kind < 0 || int(kind) >= len(names) {
		return "invalid"
	}
	return names[kind]
}

// Value is a local handle owned by a Scope, not a copied JavaScript value.
// Copying the Go wrapper is allowed, but all copies expire with the scope.
// Values cannot be moved to another scope, isolate, thread or goroutine.
type Value struct {
	scope *Scope
	id    uintptr
}

func (v Value) check() error {
	if v.scope == nil || v.id == 0 {
		return ErrInvalidValue
	}
	return v.scope.check()
}

func (v Value) Kind() (ValueKind, error) {
	if err := v.check(); err != nil {
		return 0, err
	}
	kind := v.scope.native.kind(v.id)
	if kind < 0 {
		return 0, ErrInvalidValue
	}
	return ValueKind(kind), nil
}

// Float64 reads a JavaScript Number. It does not coerce strings or BigInts.
func (v Value) Float64() (float64, error) {
	if err := v.check(); err != nil {
		return 0, err
	}
	number, ok := v.scope.native.number(v.id)
	if !ok {
		return 0, ErrType
	}
	return number, nil
}

// Bool reads a JavaScript Boolean, not JavaScript truthiness.
func (v Value) Bool() (bool, error) {
	if err := v.check(); err != nil {
		return false, err
	}
	boolean := v.scope.native.boolean(v.id)
	if boolean < 0 {
		return false, ErrType
	}
	return boolean != 0, nil
}

// ToString explicitly applies JavaScript ToString and copies UTF-8 into Go.
// This can execute user-defined JavaScript and return a JavaScript exception.
func (v Value) ToString() (string, error) {
	if err := v.check(); err != nil {
		return "", err
	}
	return v.scope.native.valueString(v.id)
}

// Call invokes a function value with an undefined receiver. All arguments must
// belong to the same active scope as the function. No JS source is compiled.
func (v Value) Call(args ...Value) (Value, error) {
	if err := v.check(); err != nil {
		return Value{}, err
	}
	var small [16]uintptr
	var ids []uintptr
	if len(args) <= len(small) {
		ids = small[:len(args)]
	} else {
		ids = make([]uintptr, len(args))
	}
	for index, arg := range args {
		if arg.scope == nil || arg.id == 0 {
			return Value{}, ErrInvalidValue
		}
		if arg.scope != v.scope {
			return Value{}, ErrWrongScope
		}
		// v.check already validated this shared scope and its owning thread.
		ids[index] = arg.id
	}
	h, err := v.scope.native.call(v.id, ids)
	return v.scope.value(h, err)
}

func (s *Scope) newValue(kind ValueKind, bits uint64, text string) (Value, error) {
	if err := s.check(); err != nil {
		return Value{}, err
	}
	h, err := s.native.newValue(kind, bits, text)
	return s.value(h, err)
}

func (s *Scope) Number(number float64) (Value, error) {
	return s.newValue(KindNumber, math.Float64bits(number), "")
}

func (s *Scope) Boolean(boolean bool) (Value, error) {
	var bits uint64
	if boolean {
		bits = 1
	}
	return s.newValue(KindBoolean, bits, "")
}

func (s *Scope) String(text string) (Value, error) {
	return s.newValue(KindString, 0, text)
}

func (s *Scope) Undefined() (Value, error) { return s.newValue(KindUndefined, 0, "") }
func (s *Scope) Null() (Value, error)      { return s.newValue(KindNull, 0, "") }
