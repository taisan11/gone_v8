//go:build ignore

// CLI for the reusable get_libs package.
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"

	"github.com/taisan11/gone_v8/get_libs"
)

func main() {
	version := flag.String("version", get_libs.DefaultVersion, "pinned V8 release tag or latest")
	target := flag.String("target", runtime.GOOS+"/"+runtime.GOARCH, "GOOS/GOARCH")
	root := flag.String("root", ".", "repository directory where deps will be installed")
	headersOnly := flag.Bool("headers-only", false, "fetch headers only")
	libOnly := flag.Bool("lib-only", false, "fetch native static library only")
	force := flag.Bool("force", false, "force redownload")
	flag.Parse()
	_, err := get_libs.Setup(get_libs.Options{Root: *root, Version: *version, Target: *target, HeadersOnly: *headersOnly, LibraryOnly: *libOnly, Force: *force})
	if err != nil {
		fmt.Fprintln(os.Stderr, "gone_v8 fetch:", err)
		os.Exit(1)
	}
	fmt.Println("Done. Only prebuilt V8 binaries and headers were installed.")
	if *target == "windows/amd64" {
		fmt.Println("Next: powershell -ExecutionPolicy Bypass -File ./tools/build-windows.ps1 -SkipFetch")
	}
}
