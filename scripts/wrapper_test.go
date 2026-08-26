//go:build !windows

// Package scripts holds no Go code. This test covers the POSIX sh
// wrapper next to it, which resolves a version and then downloads,
// verifies, and execs a release binary. Only the rejection paths are
// exercised: they fail before any network call, so the test stays
// hermetic.
package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// wrapperEnv points HOME and XDG_CACHE_HOME at cache so a real cache
// on the developer's machine cannot satisfy the run, and puts a curl
// that always fails ahead of the real one on PATH. A version that
// passes validation therefore stops at the download instead of pulling
// a release over the network, which keeps every case here offline.
func wrapperEnv(t *testing.T, cache string) []string {
	t.Helper()
	stub := t.TempDir()
	for _, name := range []string{"curl", "wget"} {
		path := filepath.Join(stub, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
			t.Fatalf("write %s stub: %v", name, err)
		}
	}
	return append(os.Environ(),
		"PATH="+stub+string(os.PathListSeparator)+os.Getenv("PATH"),
		"HOME="+cache,
		"XDG_CACHE_HOME="+filepath.Join(cache, "cache"),
	)
}

// runWrapper invokes the wrapper with DOGFETCH_VERSION set.
func runWrapper(t *testing.T, version string) (string, error) {
	t.Helper()
	cache := t.TempDir()
	cmd := exec.Command("sh", "dogfetch.sh", "--version")
	cmd.Env = append(wrapperEnv(t, cache), "DOGFETCH_VERSION="+version)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestWrapperRejectsTraversingVersion(t *testing.T) {
	// curl normalizes "../" inside a URL path, and the version lands in
	// both the release URL and the cache path. Seven of them walk off
	// /jtzemp/dogfetch/ onto another repository, whose own checksums.txt
	// would then "verify" whatever it served.
	traversal := strings.Repeat("../", 7) + "attacker/repo/releases/download/v1"
	tests := []struct {
		name    string
		version string
	}{
		{"path traversal", traversal},
		{"parent directory", "../evil"},
		{"absolute path", "/etc/passwd"},
		{"leading dash reads as a flag", "-rf"},
		{"dot dot", "a..b"},
		{"command substitution", "$(id)"},
		{"semicolon", "1.0.0;id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := runWrapper(t, tt.version)
			if err == nil {
				t.Fatalf("wrapper accepted version %q; output:\n%s", tt.version, out)
			}
			if !strings.Contains(out, "invalid version") {
				t.Errorf("version %q rejected for the wrong reason:\n%s", tt.version, out)
			}
		})
	}
}

func TestWrapperRejectsTraversingPinFile(t *testing.T) {
	// The pin file is the reachable half: a plain file under the cache
	// directory that --self-update writes and set_version reads back.
	cache := t.TempDir()
	pinDir := filepath.Join(cache, "cache", "dogfetch")
	if err := os.MkdirAll(pinDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	pin := strings.Repeat("../", 7) + "attacker/repo/releases/download/v1"
	if err := os.WriteFile(filepath.Join(pinDir, "pin"), []byte(pin+"\n"), 0o600); err != nil {
		t.Fatalf("write pin: %v", err)
	}

	cmd := exec.Command("sh", "dogfetch.sh", "--version")
	cmd.Env = append(wrapperEnv(t, cache), "DOGFETCH_VERSION=")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("wrapper accepted a traversing pin; output:\n%s", out)
	}
	if !strings.Contains(string(out), "invalid version") {
		t.Errorf("traversing pin rejected for the wrong reason:\n%s", out)
	}
}

func TestWrapperRejectsTraversingTagCache(t *testing.T) {
	// latest-tag is the third file-backed source, written whenever the
	// wrapper resolves a release and read back for 24h. Same directory,
	// same reachability as the pin.
	cache := t.TempDir()
	cacheDir := filepath.Join(cache, "cache", "dogfetch")
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	tag := "v" + strings.Repeat("../", 7) + "attacker/repo/releases/download/v1"
	if err := os.WriteFile(filepath.Join(cacheDir, "latest-tag"), []byte(tag+"\n"), 0o600); err != nil {
		t.Fatalf("write latest-tag: %v", err)
	}

	cmd := exec.Command("sh", "dogfetch.sh", "--version")
	cmd.Env = append(wrapperEnv(t, cache), "DOGFETCH_VERSION=")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("wrapper accepted a traversing tag cache; output:\n%s", out)
	}
	if !strings.Contains(string(out), "invalid version") {
		t.Errorf("traversing tag cache rejected for the wrong reason:\n%s", out)
	}
}

func TestWrapperAcceptsOrdinaryVersions(t *testing.T) {
	// These must survive validation. They still fail afterwards, at the
	// stubbed download, so assert only that the failure is not the
	// validation error.
	for _, v := range []string{"0.3.1", "v0.3.1", "1.2.3-rc.1", "10.20.30"} {
		t.Run(v, func(t *testing.T) {
			out, _ := runWrapper(t, v)
			if strings.Contains(out, "invalid version") {
				t.Errorf("wrapper rejected legitimate version %q:\n%s", v, out)
			}
		})
	}
}

// A traversing version points BIN at an executable that already exists,
// and the `[ -x "$BIN" ]` early return then skips both the download and
// the checksum before exec'ing it. This is the route that actually
// works: the download path dies earlier, because VERSION is part of the
// local archive filename too and curl cannot create it.
func TestWrapperRejectsCachedBinaryTraversal(t *testing.T) {
	cache := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cache, "cache", "dogfetch"), 0o700); err != nil {
		t.Fatalf("mkdir cache: %v", err)
	}
	planted := filepath.Join(cache, "evil")
	if err := os.MkdirAll(planted, 0o700); err != nil {
		t.Fatalf("mkdir evil: %v", err)
	}
	if err := os.WriteFile(filepath.Join(planted, "dogfetch"), []byte("#!/bin/sh\necho PWNED\n"), 0o755); err != nil {
		t.Fatalf("plant binary: %v", err)
	}

	// cache/cache/dogfetch -> cache/evil
	cmd := exec.Command("sh", "dogfetch.sh", "--version")
	cmd.Env = append(wrapperEnv(t, cache), "DOGFETCH_VERSION=../../evil")
	raw, err := cmd.CombinedOutput()
	out := string(raw)
	if strings.Contains(out, "PWNED") {
		t.Fatalf("wrapper exec'd a planted binary:\n%s", out)
	}
	if err == nil {
		t.Fatalf("wrapper accepted a traversing version:\n%s", out)
	}
	if !strings.Contains(out, "invalid version") {
		t.Errorf("rejected for the wrong reason:\n%s", out)
	}
}
