# gone_v8 — latest prebuilt V8 prototype

Module: `github.com/taisan11/gone_v8`. Import the convenience API from that path and the thread-affine low-level API from `github.com/taisan11/gone_v8/lowlevel`. The former root-level low-level names remain available as compatibility aliases.

A small Go/Cgo binding prototype inspired by `denoland/rusty_v8`, using **precompiled Google V8** from [`just-js/v8`](https://github.com/just-js/v8/releases). **V8 is never compiled on your machine.**

## Releases and dependency setup

Pushing a `v*` tag runs `.github/workflows/release.yml`: it downloads the pinned prebuilt V8, builds the Windows bridge where applicable and the demos, and publishes archives containing source, native dependencies, and demo binaries. It does not run tests or `go vet`. Manual workflow runs build downloadable artifacts but do not publish a release unless run on a `v*` tag. Release targets are Linux and macOS amd64/arm64 and Windows amd64; a release is published only after every matrix build succeeds.

Use a target-specific release archive as a writable source checkout, or use the [`get_libs` utility API](get_libs/README.md) / `go run ./tools/fetch.go` to install dependencies in a checkout. Native files are not automatically installed by `go get`: for a checkout used by another module, use a Go workspace or `replace`. Windows additionally requires a built bridge DLL at runtime.

## Version selection

- **Default:** `just-js/v8` release **15.3**, published on **2026-10-09**. This release's own workflow checks out V8 `branch-heads/15.3` (the release notes mistakenly say 15.2).
- **Follow releases:** `-version latest` asks GitHub for the latest `just-js/v8` release and downloads its published, SHA-256 verified assets. If no compatible binary exists, setup fails rather than compiling V8 from source.
- **Reproducible builds:** pin a version and validate toolchain and V8 ABI whenever updating. The source API must still be updated when V8 changes incompatibly.

## Quick start — Linux or macOS

Requirements: Go >= 1.22, a **C++20-capable toolchain**, and CGO enabled. You do **not** need GN, Ninja, depot_tools or the V8 source tree.

```bash
cd gone_v8_latest

# Fetch same-version headers + prebuilt V8 library (defaults to 15.3)
go run ./tools/fetch.go

# Or opt into latest release offered by just-js/v8
go run ./tools/fetch.go -version latest

# Compile only the tiny local C++ bridge, not V8:
CGO_ENABLED=1 go run ./cmd/demo
CGO_ENABLED=1 go test ./...
```

On Linux the provider built V8 with **system libstdc++** (`use_custom_libcxx=false`), unlike the old robomotionio/v8go prebuilt V8. With GCC/g++ use `CC=gcc CXX=g++`; with Clang use a compatible libstdc++ setup. On macOS, use Apple Clang/libc++.

The downloader verifies the *compressed* GitHub asset's SHA-256 before extracting it. For the default release SHA-256 values are pinned in `tools/fetch.go`; other versions require metadata from the GitHub Releases API. The tool does not silently build V8 if assets are missing.

## Quick start — Windows x64

Windows uses a small **C ABI DLL**, built with clang-cl against the prebuilt V8 archive and its matching Chromium libc++. Go loads the DLL directly, so **Cgo and MinGW are not required on Windows**. V8 itself is still never compiled locally.

Build requirements:

- Go >= 1.22 (windows/amd64)
- Visual Studio C++ Build Tools, including the x64 toolchain and Windows SDK
- LLVM **clang-cl**, available as the optional **C++ Clang tools for Windows** component in Visual Studio Installer. The script also searches Visual Studio's LLVM directories when clang-cl is not on PATH. Older Clang versions may be insufficient; use `-Clang` to select a newer LLVM installation.

Open an **x64 Visual Studio Developer PowerShell**, then run:

```powershell
# Downloads V8 + matching libc++ headers and compiles only the bridge DLL:
./tools/build-windows.ps1
# Select a separate LLVM installation if needed:
# ./tools/build-windows.ps1 -Clang 'C:\Program Files\LLVM\bin\clang-cl.exe'

$env:CGO_ENABLED = '0'
go test ./...
go run ./cmd/demo
```

If script execution is restricted, invoke it with `powershell -ExecutionPolicy Bypass -File ./tools/build-windows.ps1` in the developer environment.

To use a different V8 release, first run `go run ./tools/fetch.go -version <tag> -target windows/amd64`, then `./tools/build-windows.ps1 -SkipFetch`. V8 headers, the static library and libc++ headers must all have the same version. Without `-SkipFetch`, the build script fetches the pinned default version.

The output is `deps/windows_amd64/gone_v8.dll`. **Deploy this DLL next to your application's executable**, or set `$env:GONE_V8_DLL` to its full path. During development, `go run` / `go test` also look in this checkout's `deps/windows_amd64` directory (not reliable with `-trimpath`). No DLL is loaded from the current working directory by default. An explicit `GONE_V8_DLL` overrides all other locations. The DLL stays loaded for the process lifetime; restart the application after replacing it.

**Verified on Windows x64:** native DLL compilation, `go test ./...`, `go vet ./...`, dependency extraction tests and `go run ./cmd/demo`, using Go 1.27.0, Visual Studio Build Tools 2026 (MSVC 14.51) and clang-cl 22.1.3. Tests cover evaluation, persistent contexts, Unicode/NUL strings, JavaScript errors, independent isolates, closing engines and concurrent callers.

The bridge uses V8's bundled Chromium libc++, not MSVC's STL. Its exception support still needs MSVC's `__ExceptionPtr*` implementation. The build script extracts only `excptptr.obj` from the installed `libcpmt.lib` and disables automatic linkage of that archive, avoiding duplicate `std::nothrow` / `std::uncaught_exceptions` definitions. It does not modify the installed runtime libraries or ignore duplicate-symbol errors.

## Target support

| Target | Download | Prototype native link |
|---|---|---|
| Linux amd64 | yes | configured, **not end-to-end tested** |
| Linux arm64 | yes | configured, **not end-to-end tested** |
| macOS amd64 | yes | configured, **not end-to-end tested** |
| macOS arm64 | yes | configured, **not end-to-end tested** |
| Windows amd64 | yes | **end-to-end tested** with the toolchain above |
| Windows arm64 | no upstream artifact | no |

To fetch a different platform's assets without running them, use `go run ./tools/fetch.go -target windows/amd64`. For Windows this downloads the `.lib` and matching libc++ headers; build the DLL on Windows using the script above.

**Important V8 build characteristics:** the provider disables ICU (`v8_enable_i18n_support=false`), disables V8 Sandbox (`v8_enable_sandbox=false`), enables pointer compression (`v8_enable_pointer_compression=true`), and has its own C++ runtime configuration. This library is not equivalent to a stock Chromium build, and should **not** be presented as a standalone security sandbox for untrusted code. In particular ICU-dependent `Intl` functionality is missing.

## Main API

```go
package main

import (
    "fmt"
    "log"

    gone_v8 "github.com/taisan11/gone_v8"
)

func main() {
    engine, err := gone_v8.New()
    if err != nil { log.Fatal(err) }
    defer engine.Close()

    value, err := engine.Eval("1 + 2 * 3")
    if err != nil { log.Fatal(err) }
    fmt.Println(value) // 7
}
```

`Eval` compiles and executes source in the engine's persistent JavaScript context, then converts the result with JavaScript `ToString`. JavaScript exceptions are returned as Go errors. Calls from multiple goroutines are safe and serialized on the engine's worker thread. `Close` releases the engine; calls after closing return `ErrClosed`.

### Compile once and run repeatedly

```go
script, err := engine.Compile("6 * 7")
if err != nil { return err }
defer script.Close()

text, err := script.Run() // "42"; no source compilation on each Run
```

`CompiledScript` is goroutine-safe: its Engine serializes `Run` and `Close`. `Run` still dispatches to the worker and converts the result to a Go string. Compiling does not execute the source. Closing the Engine also releases all of its compiled scripts.

## Low-level API

For thread-affine, typed V8 operations and batching, see the [low-level API documentation](lowlevel/README.md).

## Currently implemented

- One V8 Isolate and retained Context per `Engine`
- Serialized access on a locked OS thread
- Compile-once scripts for repeated execution
- Additional typed and batched operations in the [low-level API](lowlevel/README.md)
- JavaScript exceptions propagated as Go errors

**Not yet implemented:** full rusty_v8 API compatibility, transferable/persistent value handles, custom contexts, JS object property APIs, Go-to-JS callbacks, modules, inspector, snapshots, serialized code caches. Native V8 assets are fetched from the upstream publisher. Windows DLL linking and execution were tested as described above; Linux/macOS linking remains unverified in this environment.

## Licensing / sources

- https://github.com/just-js/v8/releases/tag/15.3
- https://github.com/denoland/rusty_v8
- https://v8.dev/

Example Go/C++ bridge is MIT-licensed. Review V8, Chromium and third-party notices when redistributing.
