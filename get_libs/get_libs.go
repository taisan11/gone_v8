// Package get_libs installs prebuilt V8 libraries and matching headers.
// It is independent of the V8 binding and can be used before compiling it.
package get_libs

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	repository    = "just-js/v8"
	pinnedVersion = "15.3"
	baseURL       = "https://github.com/just-js/v8/releases/download/"
	maxDownload   = 200 << 20
	maxHeader     = 16 << 20
	maxLibrary    = 1 << 30
)

type asset struct {
	Name   string `json:"name"`
	Digest string `json:"digest"`
	URL    string `json:"browser_download_url"`
}

type release struct {
	Tag    string  `json:"tag_name"`
	Assets []asset `json:"assets"`
}

// Trusted SHA256s from the just-js/v8 15.3 release metadata (2026-10-09).
// No dependency on the GitHub API for a fixed-version setup.
var pinnedSHA = map[string]string{
	"include.tar.gz":                  "d13ef957bd96be0e629248d47dbdad16a0fb70d31c82fe6689149eb6c3887aab",
	"libv8_monolith-linux-x64.a.gz":   "b5971207269a4c101ad0d7ac399dea737487c5a6a737d82eac11f004968f815b",
	"libv8_monolith-linux-arm64.a.gz": "c615d4bd6ab7ad77a598973a7ca5de801462f30c4e36236e08cfc2c53bdea6b2",
	"libv8_monolith-mac-x64.a.gz":     "d33a64bb2de5f4e9864f56202c72a6c97e668a46d406f4c88377c5e8912cfc23",
	"libv8_monolith-mac-arm64.a.gz":   "2e0e20f3f6d8753b119131f58d221759d7e0913e53d6a883ca4f21967bb3a6ed",
	"libv8_monolith-win-x64.zip":      "1b6756bde9bbe2c3e00b8d0137e7172d5c42c27c1884b2a8876af59387c41a93",
	"libcxx-headers-win-x64.zip":      "9f1a0668e365aa6482bf70412aa2faf013257aae3af29917af03007226ab4873",
}

var httpClient = &http.Client{Timeout: 10 * time.Minute}

// Options controls installation under Root/deps. Zero values use the current
// directory, the pinned version and the current GOOS/GOARCH. Setup must not run
// concurrently with another installation into the same directory or a build.
type Options struct {
	Root        string
	Version     string
	Target      string
	HeadersOnly bool
	LibraryOnly bool
	Force       bool
}

// DefaultVersion is the upstream release used when Options.Version is empty.
const DefaultVersion = pinnedVersion

// Setup downloads and verifies matching prebuilt V8 assets. It returns the
// resolved upstream release tag. On Windows it fetches build inputs only;
// building the bridge DLL requires the separate Windows build script.
func Setup(options Options) (string, error) {
	if options.HeadersOnly && options.LibraryOnly {
		return "", errors.New("headers-only and library-only are mutually exclusive")
	}
	version := options.Version
	if version == "" {
		version = pinnedVersion
	}
	target := options.Target
	if target == "" {
		target = runtime.GOOS + "/" + runtime.GOARCH
	}
	root := options.Root
	if root == "" {
		root = "."
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	libAsset, err := chooseLibrary(target)
	if err != nil {
		return "", err
	}

	var rel release
	if version == pinnedVersion {
		rel.Tag = pinnedVersion
		for name, sha := range pinnedSHA {
			rel.Assets = append(rel.Assets, asset{Name: name, Digest: "sha256:" + sha, URL: baseURL + pinnedVersion + "/" + name})
		}
	} else {
		endpoint := "https://api.github.com/repos/" + repository + "/releases/tags/" + version
		if version == "latest" {
			endpoint = "https://api.github.com/repos/" + repository + "/releases/latest"
		}
		resp, err := httpClient.Get(endpoint)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return "", fmt.Errorf("GitHub release metadata: HTTP %d", resp.StatusCode)
		}
		if err := json.NewDecoder(io.LimitReader(resp.Body, 5<<20)).Decode(&rel); err != nil {
			return "", err
		}
		if rel.Tag == "" {
			return "", errors.New("release missing tag")
		}
	}
	fmt.Printf("gone_v8: selected just-js/v8 release %s for %s\n", rel.Tag, target)
	if !options.LibraryOnly {
		if err := fetchHeaders(root, rel, options.Force); err != nil {
			return "", err
		}
		if target == "windows/amd64" {
			if err := fetchWindowsLibcxx(root, rel, options.Force); err != nil {
				return "", err
			}
		}
	}
	if !options.HeadersOnly {
		if err := fetchLibrary(root, rel, target, libAsset, options.Force); err != nil {
			return "", err
		}
	}
	return rel.Tag, nil
}

func chooseLibrary(target string) (string, error) {
	switch target {
	case "linux/amd64":
		return "libv8_monolith-linux-x64.a.gz", nil
	case "linux/arm64":
		return "libv8_monolith-linux-arm64.a.gz", nil
	case "darwin/amd64":
		return "libv8_monolith-mac-x64.a.gz", nil
	case "darwin/arm64":
		return "libv8_monolith-mac-arm64.a.gz", nil
	case "windows/amd64":
		return "libv8_monolith-win-x64.zip", nil
	default:
		return "", fmt.Errorf("unsupported prebuilt target %s (Windows ARM64 is not published by this provider)", target)
	}
}

func findAsset(rel release, name string) (asset, error) {
	for _, a := range rel.Assets {
		if a.Name == name {
			if a.Digest == "" || !strings.HasPrefix(a.Digest, "sha256:") {
				return a, fmt.Errorf("asset %s lacks sha256 digest", name)
			}
			if a.URL != baseURL+rel.Tag+"/"+name {
				return a, fmt.Errorf("unexpected download URL for %s", name)
			}
			digest, err := hex.DecodeString(strings.TrimPrefix(a.Digest, "sha256:"))
			if err != nil || len(digest) != sha256.Size {
				return a, fmt.Errorf("invalid SHA256 of %s", name)
			}
			return a, nil
		}
	}
	return asset{}, fmt.Errorf("release %s doesn't publish %s", rel.Tag, name)
}

func download(a asset) (string, error) {
	resp, err := httpClient.Get(a.URL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("download %s: HTTP %d", a.Name, resp.StatusCode)
	}
	file, err := os.CreateTemp("", "gone_v8_*.download")
	if err != nil {
		return "", err
	}
	name := file.Name()
	h := sha256.New()
	size, err := io.Copy(io.MultiWriter(file, h), io.LimitReader(resp.Body, maxDownload+1))
	closeErr := file.Close()
	if err != nil || closeErr != nil || size > maxDownload {
		os.Remove(name)
		return "", fmt.Errorf("download %s failed/too large: %v %v", a.Name, err, closeErr)
	}
	got := hex.EncodeToString(h.Sum(nil))
	want := strings.TrimPrefix(a.Digest, "sha256:")
	if !strings.EqualFold(got, want) {
		os.Remove(name)
		return "", fmt.Errorf("SHA256 mismatch for %s: want %s, got %s", a.Name, want, got)
	}
	return name, nil
}

func fetchHeaders(root string, rel release, force bool) error {
	marker := filepath.Join(root, "deps", ".headers-version")
	if !force {
		if b, err := os.ReadFile(marker); err == nil && strings.TrimSpace(string(b)) == rel.Tag {
			if _, err := os.Stat(filepath.Join(root, "deps", "include", "v8.h")); err == nil {
				fmt.Println("Cached V8 headers", rel.Tag)
				return nil
			}
		}
	}
	a, err := findAsset(rel, "include.tar.gz")
	if err != nil {
		return err
	}
	fmt.Println("Downloading:", a.Name)
	path, err := download(a)
	if err != nil {
		return err
	}
	defer os.Remove(path)
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	count := 0
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag != tar.TypeReg && hdr.Typeflag != tar.TypeRegA {
			continue
		}
		clean := filepath.Clean(filepath.FromSlash(hdr.Name))
		if clean == "include" || filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") || !strings.HasPrefix(clean, "include"+string(filepath.Separator)) {
			return fmt.Errorf("bad header path: %q", hdr.Name)
		}
		if hdr.Size < 0 || hdr.Size > maxHeader {
			return fmt.Errorf("header too large: %q", hdr.Name)
		}
		dest := filepath.Join(root, "deps", clean)
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return err
		}
		out, err := os.Create(dest)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, io.LimitReader(tr, hdr.Size))
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		count++
	}
	if count == 0 {
		return errors.New("archive has no headers")
	}
	if _, err := os.Stat(filepath.Join(root, "deps", "include", "v8.h")); err != nil {
		return fmt.Errorf("archive did not contain include/v8.h: %w", err)
	}
	if err := os.WriteFile(marker, []byte(rel.Tag+"\n"), 0644); err != nil {
		return err
	}
	fmt.Printf("Installed %d headers\n", count)
	return nil
}

func fetchLibrary(root string, rel release, target, name string, force bool) error {
	a, err := findAsset(rel, name)
	if err != nil {
		return err
	}
	path := filepath.Join(root, "deps", strings.ReplaceAll(target, "/", "_"), "libv8.a")
	if strings.HasPrefix(target, "windows/") {
		path = filepath.Join(root, "deps", strings.ReplaceAll(target, "/", "_"), "libv8.lib")
	}
	marker := path + ".version"
	if !force {
		if b, err := os.ReadFile(marker); err == nil && strings.TrimSpace(string(b)) == rel.Tag {
			if stat, err := os.Stat(path); err == nil && stat.Size() > 0 {
				fmt.Println("Cached library:", path)
				return nil
			}
		}
	}
	fmt.Println("Downloading:", a.Name)
	tmp, err := download(a)
	if err != nil {
		return err
	}
	defer os.Remove(tmp)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	out, err := os.CreateTemp(filepath.Dir(path), ".gone_v8_lib_*")
	if err != nil {
		return err
	}
	defer os.Remove(out.Name())
	if strings.HasSuffix(name, ".a.gz") {
		file, err := os.Open(tmp)
		if err != nil {
			out.Close()
			return err
		}
		gz, err := gzip.NewReader(file)
		if err != nil {
			file.Close()
			out.Close()
			return err
		}
		n, err := io.Copy(out, io.LimitReader(gz, maxLibrary+1))
		gz.Close()
		file.Close()
		if err != nil || n > maxLibrary {
			out.Close()
			return fmt.Errorf("decompress library: %v", err)
		}
	} else {
		z, err := zip.OpenReader(tmp)
		if err != nil {
			out.Close()
			return err
		}
		found := false
		for _, entry := range z.File {
			if entry.Name != "v8_monolith.lib" {
				continue
			}
			if entry.UncompressedSize64 > maxLibrary {
				z.Close()
				out.Close()
				return errors.New("Windows .lib too large")
			}
			reader, err := entry.Open()
			if err != nil {
				z.Close()
				out.Close()
				return err
			}
			_, err = io.Copy(out, io.LimitReader(reader, maxLibrary+1))
			reader.Close()
			if err != nil {
				z.Close()
				out.Close()
				return err
			}
			found = true
			break
		}
		z.Close()
		if !found {
			out.Close()
			return errors.New("Windows archive has no v8_monolith.lib")
		}
	}
	if err := out.Close(); err != nil {
		return err
	}
	if err := os.Rename(out.Name(), path); err != nil {
		return err
	}
	if err := os.WriteFile(marker, []byte(rel.Tag+"\n"), 0644); err != nil {
		return err
	}
	fmt.Println("Installed:", path)
	return nil
}

// libc++ headers must match the V8 archive's Chromium ABI, not the host STL.
func fetchWindowsLibcxx(root string, rel release, force bool) error {
	dest := filepath.Join(root, "deps", "windows_amd64", "libcxx")
	marker := filepath.Join(dest, ".version")
	required := []string{
		"third_party/libc++/src/include/__config",
		"buildtools/third_party/libc++/__config_site",
		"buildtools/third_party/libc++/__assertion_handler",
	}
	if !force {
		if b, err := os.ReadFile(marker); err == nil && strings.TrimSpace(string(b)) == rel.Tag {
			complete := true
			for _, name := range required {
				if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(name))); err != nil {
					complete = false
				}
			}
			if complete {
				fmt.Println("Cached Windows libc++ headers", rel.Tag)
				return nil
			}
		}
	}
	a, err := findAsset(rel, "libcxx-headers-win-x64.zip")
	if err != nil {
		return err
	}
	fmt.Println("Downloading:", a.Name)
	path, err := download(a)
	if err != nil {
		return err
	}
	defer os.Remove(path)
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Dir(dest), ".libcxx-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err := extractLibcxx(path, stage); err != nil {
		return err
	}
	for _, name := range required {
		if _, err := os.Stat(filepath.Join(stage, filepath.FromSlash(name))); err != nil {
			return fmt.Errorf("libc++ archive missing %s: %w", name, err)
		}
	}
	if err := os.WriteFile(filepath.Join(stage, ".version"), []byte(rel.Tag+"\n"), 0644); err != nil {
		return err
	}
	if err := os.RemoveAll(dest); err != nil {
		return err
	}
	return os.Rename(stage, dest)
}

func extractLibcxx(path, dest string) error {
	z, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer z.Close()
	var total uint64
	for _, entry := range z.File {
		// PowerShell archives may use backslashes. Validate both separators on
		// every OS so -target windows/amd64 is safe on Unix as well.
		name := strings.ReplaceAll(entry.Name, "\\", "/")
		clean := filepath.Clean(filepath.FromSlash(name))
		if strings.Contains(name, ":") || strings.HasPrefix(name, "/") || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || filepath.IsAbs(clean) {
			return fmt.Errorf("bad libc++ header path: %q", entry.Name)
		}
		if entry.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("libc++ archive contains symlink: %q", entry.Name)
		}
		if entry.FileInfo().IsDir() {
			continue
		}
		if entry.UncompressedSize64 > maxHeader {
			return fmt.Errorf("libc++ header too large: %q", entry.Name)
		}
		total += entry.UncompressedSize64
		if total > maxDownload {
			return errors.New("libc++ headers too large")
		}
		outPath := filepath.Join(dest, clean)
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			return err
		}
		reader, err := entry.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(outPath)
		if err != nil {
			reader.Close()
			return err
		}
		n, copyErr := io.Copy(out, io.LimitReader(reader, maxHeader+1))
		reader.Close()
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if n > maxHeader {
			return errors.New("libc++ header too large")
		}
	}
	return nil
}
