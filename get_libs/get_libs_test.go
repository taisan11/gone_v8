package get_libs

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestChooseLibrary(t *testing.T) {
	cases := map[string]string{
		"linux/amd64":   "libv8_monolith-linux-x64.a.gz",
		"linux/arm64":   "libv8_monolith-linux-arm64.a.gz",
		"darwin/amd64":  "libv8_monolith-mac-x64.a.gz",
		"darwin/arm64":  "libv8_monolith-mac-arm64.a.gz",
		"windows/amd64": "libv8_monolith-win-x64.zip",
	}
	for target, want := range cases {
		got, err := chooseLibrary(target)
		if err != nil || got != want {
			t.Errorf("%s = %q, %v", target, got, err)
		}
	}
	if _, err := chooseLibrary("windows/arm64"); err == nil {
		t.Fatal("Windows ARM64 must be unavailable")
	}
}

func TestExtractLibcxx(t *testing.T) {
	for _, name := range []string{
		"third_party/libc++/src/include/vector",
		`buildtools\third_party\libc++\__config_site`,
		"../escape", `..\escape`, "/absolute", `C:\escape`,
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			archive := filepath.Join(root, "headers.zip")
			file, err := os.Create(archive)
			if err != nil {
				t.Fatal(err)
			}
			z := zip.NewWriter(file)
			entry, err := z.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := entry.Write([]byte("header")); err != nil {
				t.Fatal(err)
			}
			if err := z.Close(); err != nil {
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			dest := filepath.Join(root, "out")
			err = extractLibcxx(archive, dest)
			valid := name == "third_party/libc++/src/include/vector" || name == `buildtools\third_party\libc++\__config_site`
			if valid {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatalf("unsafe path accepted: %q", name)
			}
		})
	}
}

func TestRejectShortDigest(t *testing.T) {
	r := release{Tag: pinnedVersion, Assets: []asset{{
		Name: "include.tar.gz", Digest: "sha256:1234",
		URL: baseURL + pinnedVersion + "/include.tar.gz",
	}}}
	if _, err := findAsset(r, "include.tar.gz"); err == nil {
		t.Fatal("short digest accepted")
	}
}

func TestPinnedAssetManifest(t *testing.T) {
	r := release{Tag: pinnedVersion}
	for name, sha := range pinnedSHA {
		r.Assets = append(r.Assets, asset{Name: name, Digest: "sha256:" + sha, URL: baseURL + pinnedVersion + "/" + name})
	}
	for name := range pinnedSHA {
		if _, err := findAsset(r, name); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	bad := release{Tag: "15.3", Assets: []asset{{Name: "include.tar.gz", Digest: "sha256:1234", URL: "https://untrusted.invalid/bad"}}}
	if _, err := findAsset(bad, "include.tar.gz"); err == nil {
		t.Fatal("unexpected URL should be rejected")
	}
}
