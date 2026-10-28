# Low-level API

Import `github.com/taisan11/gone_v8/lowlevel` for thread-affine V8 operations. For the goroutine-safe main API and build instructions, see the [root README](../README.md).

## Direct execution with typed results

```go
package main

import (
    "fmt"
    "log"

    "github.com/taisan11/gone_v8/lowlevel"
)

func main() {
    err := lowlevel.WithIsolate(func(isolate *lowlevel.Isolate) error {
        script, err := isolate.Compile("6 * 7")
        if err != nil { return err }
        defer script.Close()
        return isolate.WithScope(func(scope *lowlevel.Scope) error {
            value, err := script.Run(scope)
            if err != nil { return err }
            number, err := value.Float64()
            if err != nil { return err }
            fmt.Println(number) // 42
            return nil
        })
    })
    if err != nil { log.Fatal(err) }
}
```

`WithIsolate` pins the calling goroutine to its OS thread for the callback, then frees the isolate and its resources. It does not use a worker or channels. See also `go run ./cmd/lowlevel` from the repository root.

For an existing main API `Engine`, `engine.Do(func(isolate *lowlevel.Isolate) error { ... })` makes one worker round-trip for the whole callback. A `Script` can be reused in subsequent `Do` callbacks on the same Engine; close it inside `Do` or let Engine teardown free it.

## Operations

| API | Behavior |
|---|---|
| `Isolate.WithScope(fn)` | Open a V8 HandleScope; cleanup also runs on panic |
| `Isolate.Compile(source)` / `Scope.Compile(source)` | Compile without executing, producing a persistent context-bound `Script` |
| `Script.Run(scope)` | Execute compiled code and return a local `Value` |
| `Scope.Eval(source)` | Compile and execute, returning a local `Value` |
| `Value.Kind()` | Inspect undefined/null/boolean/number/string/object/function/bigint/symbol |
| `Value.Float64()` / `Value.Bool()` | Strict typed reads; `ErrType` instead of coercion |
| `Value.ToString()` | Explicit JS ToString and UTF-8 copy; may throw |
| `Scope.Number/Boolean/String/Undefined/Null` | Create local values |
| `Value.Call(args...)` | Call a function, with undefined receiver |
| `Script.Close()` | Release a persistent script on its owning thread |

## Lifetimes and safety

- `Isolate`, `Scope`, `Script` and `Value` are thread-affine and not goroutine-safe. Do not launch goroutines to use them from a callback. Wrong OS threads and copied Isolate/Script wrappers are rejected.
- A `Value` is a local handle and expires at the end of its `WithScope` callback. Copying it does not extend its lifetime. Values cannot be transferred between scopes or isolates.
- Nested scopes are supported, but only the innermost native scope can be used, including when nesting isolates. The parent becomes usable again after the child exits.
- Use short scopes or bounded batches: local handles are retained until the scope closes, so an unlimited loop in one scope can grow native memory.
- A `Script` survives scopes but is tied to its original isolate and context. Close it while the isolate is alive; remaining scripts are freed on teardown. It is not a serialized code cache.
- Inside `Engine.Do`, use the supplied Isolate rather than calling that Engine's queued methods or `CompiledScript.Run/Close`. Reentrant queued calls return `ErrReentrant`; reentrant `Engine.Close` panics. Waiting on another goroutine that needs the Engine can still deadlock.
- Callback errors propagate normally. Scope/isolate cleanup runs on panic; `Engine.Do` rethrows the panic in its caller and keeps the Engine usable. `runtime.Goexit` in `Do` shuts down the worker; subsequent operations return `ErrClosed`.
- Re-running a script uses the same context. Normal JavaScript rules apply, including top-level `let`/`const` redeclaration errors.

## Benchmarks

From the repository root:

```sh
go test -run '^$' -bench 'Benchmark(Compiled|EngineCompiled|EngineDoCompiled|ValueCall|ValueOperations)' -benchmem -count=3 ./lowlevel
```

Direct and batched benchmarks close scopes every 256 executions, amortizing scope setup and, for `Engine.Do`, dispatch. `BenchmarkValueCall` also amortizes function setup. Go allocation statistics do **not** count native allocations; small per-run allocations can also round to zero. These are end-to-end operation timings, not isolated FFI overhead or a rusty_v8 comparison.

The current Windows DLL must export `gone_v8_scope_check`. After a bridge update, rebuild it with `./tools/build-windows.ps1 -SkipFetch` and redeploy it alongside the executable.
