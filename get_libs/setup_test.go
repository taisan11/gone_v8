package get_libs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetupCachedAssets(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"deps/.headers-version":            DefaultVersion + "\n",
		"deps/include/v8.h":                "header",
		"deps/linux_amd64/libv8.a.version": DefaultVersion + "\n",
		"deps/linux_amd64/libv8.a":         "library",
	}
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	tag, err := Setup(Options{Root: root, Target: "linux/amd64"})
	if err != nil || tag != DefaultVersion {
		t.Fatalf("Setup = %q, %v", tag, err)
	}
}

func TestSetupInvalidOptions(t *testing.T) {
	for _, options := range []Options{
		{HeadersOnly: true, LibraryOnly: true},
		{Target: "windows/arm64"},
	} {
		options.Root = t.TempDir()
		if _, err := Setup(options); err == nil {
			t.Errorf("Setup(%+v) unexpectedly succeeded", options)
		}
	}
}
