# get_libs

`github.com/taisan11/gone_v8/get_libs` installs the matching prebuilt V8 headers and library into a **writable gone_v8 source checkout**. It does not import the V8 binding, so it can be built and run before the native dependencies exist. V8 itself is not compiled.

```go
package main

import (
    "log"

    "github.com/taisan11/gone_v8/get_libs"
)

func main() {
    tag, err := get_libs.Setup(get_libs.Options{
        Root: "/path/to/gone_v8", // checkout whose lowlevel package will be built
        // Version: "15.3",       // default; "latest" also accepted
        // Target: "linux/amd64", // default: current GOOS/GOARCH
    })
    if err != nil { log.Fatal(err) }
    log.Println("installed upstream V8", tag)
}
```

`Root` defaults to the working directory. `Version` defaults to `get_libs.DefaultVersion`; `Target` defaults to the current platform. Supported targets are `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, and `windows/amd64`. `Force` redownloads; `HeadersOnly` and `LibraryOnly` select a partial installation and are mutually exclusive. Partial installations do not ensure that previously installed assets match: use the same version for all components.

The default release uses pinned SHA-256 digests; other versions obtain digests from GitHub release metadata. Archives are checked before extraction. Matching version markers allow cached assets to be reused, but cached files are not rehashed on every run. Do not call `Setup` concurrently for the same directory or while compiling against it. A failed installation may leave partial files; retry it before building.

For Windows, `Setup` fetches the static library and Chromium libc++ headers, **not** the bridge DLL. On Windows run `tools/build-windows.ps1 -SkipFetch` in an x64 Visual Studio Developer environment afterwards. Deploy the resulting DLL next to the executable or set `GONE_V8_DLL`.

The CLI wrapper from the checkout is equivalent:

```sh
go run ./tools/fetch.go -root . -target linux/amd64
```

This API installs under `Root/deps`; it does not modify the Go module cache, automatically install dependencies during `go get`, or install runtime dependencies into an already built application. To use a checkout from another Go module, configure `replace github.com/taisan11/gone_v8 => /path/to/gone_v8` (or use a workspace). Release archives include source and the native assets for their target.
