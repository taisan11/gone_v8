//go:build windows && amd64

package lowlevel

import (
	"path/filepath"
	"testing"
	"unsafe"
)

func TestWindowsResultLayout(t *testing.T) {
	var r windowsResult
	if unsafe.Sizeof(r) != 24 || unsafe.Offsetof(r.length) != 8 || unsafe.Offsetof(r.isError) != 16 {
		t.Fatal("gone_v8_result C ABI layout mismatch")
	}
}

func TestWindowsBridgeOverride(t *testing.T) {
	t.Setenv("GONE_V8_DLL", `C:\custom path\gone_v8.dll`)
	paths := bridgeCandidates()
	if len(paths) != 1 || paths[0] != `C:\custom path\gone_v8.dll` {
		t.Fatalf("override must be exclusive: %v", paths)
	}
}

func TestWindowsBridgeDefaultPaths(t *testing.T) {
	t.Setenv("GONE_V8_DLL", "")
	paths := bridgeCandidates()
	if len(paths) == 0 {
		t.Fatal("no DLL paths")
	}
	for _, path := range paths {
		if !filepath.IsAbs(path) || filepath.Base(path) != "gone_v8.dll" {
			t.Fatalf("DLL lookup must use explicit absolute paths: %q", path)
		}
	}
}
