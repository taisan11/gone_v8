//go:build (cgo && (linux || darwin)) || (windows && amd64)

// Package gone_v8 provides a goroutine-safe V8 engine. The lowlevel package
// exposes thread-affine primitives for batching and direct execution.
package gone_v8

import "github.com/taisan11/gone_v8/lowlevel"

type Engine = lowlevel.Engine
type CompiledScript = lowlevel.CompiledScript

// These aliases preserve compatibility with earlier releases. New code may
// import the lowlevel package directly.
type Isolate = lowlevel.Isolate
type Scope = lowlevel.Scope
type Script = lowlevel.Script
type Value = lowlevel.Value
type ValueKind = lowlevel.ValueKind

const (
	KindUndefined = lowlevel.KindUndefined
	KindNull      = lowlevel.KindNull
	KindBoolean   = lowlevel.KindBoolean
	KindNumber    = lowlevel.KindNumber
	KindString    = lowlevel.KindString
	KindObject    = lowlevel.KindObject
	KindFunction  = lowlevel.KindFunction
	KindBigInt    = lowlevel.KindBigInt
	KindSymbol    = lowlevel.KindSymbol
)

var (
	ErrClosed          = lowlevel.ErrClosed
	ErrReentrant       = lowlevel.ErrReentrant
	ErrWrongThread     = lowlevel.ErrWrongThread
	ErrScopeClosed     = lowlevel.ErrScopeClosed
	ErrScopeNotCurrent = lowlevel.ErrScopeNotCurrent
	ErrWrongIsolate    = lowlevel.ErrWrongIsolate
	ErrWrongScope      = lowlevel.ErrWrongScope
	ErrInvalidValue    = lowlevel.ErrInvalidValue
	ErrType            = lowlevel.ErrType
	ErrCopiedResource  = lowlevel.ErrCopiedResource
)

func New() (*Engine, error)                     { return lowlevel.New() }
func WithIsolate(fn func(*Isolate) error) error { return lowlevel.WithIsolate(fn) }
