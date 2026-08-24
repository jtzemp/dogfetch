package cmd

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// combinedHandler answers both the list endpoint (fetch, patterns) and
// the aggregate endpoint (summary) so one server covers every
// subcommand's success path.
func combinedHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v2/logs/analytics/aggregate":
			_, _ = io.WriteString(w, `{"data":{"buckets":[
				{"by":{"status":"__total__"},"computes":{"c0":2,"c1":1}},
				{"by":{"status":"error"},"computes":{"c0":2}}
			]}}`)
		default:
			_, _ = io.WriteString(w, mockLogs(2, ""))
		}
	}
}

// runExecuteCapture sets os.Args and runs Execute(), capturing stdout.
func runExecuteCapture(t *testing.T, args ...string) (string, int) {
	t.Helper()
	origArgs := os.Args
	os.Args = append([]string{"dogfetch"}, args...)
	defer func() { os.Args = origArgs }()

	return captureStdout(t, Execute)
}

// clearAuth wipes credentials and points HOME at an empty dir so the
// ~/.config/dogfetch/env fallback can't supply real keys.
func clearAuth(t *testing.T) {
	t.Helper()
	t.Setenv("DD_API_KEY", "")
	t.Setenv("DD_APP_KEY", "")
	empty := t.TempDir()
	t.Setenv("HOME", empty)
	t.Setenv("USERPROFILE", empty) // os.UserHomeDir() uses USERPROFILE on Windows
	t.Setenv("XDG_CONFIG_HOME", empty)
}

func TestExitCodeMatrix(t *testing.T) {
	srv := httptest.NewServer(combinedHandler())
	defer srv.Close()

	// withServer cases run against the mock with valid auth.
	withServer := []struct {
		name string
		args []string
		want int
	}{
		{"version", []string{"version"}, exitOK},
		{"home (bare)", nil, exitOK},
		{"auth", []string{"auth"}, exitOK},
		{"fetch help", []string{"fetch", "-h"}, exitOK},
		{"fetch success", []string{"fetch", "--query", "service:web"}, exitOK},
		{"summary success", []string{"summary", "--query", "service:web"}, exitOK},
		{"patterns success", []string{"patterns", "--query", "service:web"}, exitOK},

		{"unknown command", []string{"bogus"}, exitUsage},
		{"fetch missing query", []string{"fetch"}, exitUsage},
		{"fetch bad flag", []string{"fetch", "--nope"}, exitUsage},
		{"fetch bad time", []string{"fetch", "--query", "x", "--from", "nonsense"}, exitUsage},
		{"fetch negative limit", []string{"fetch", "--query", "x", "--limit", "-1"}, exitUsage},
		{"fetch bad format", []string{"fetch", "--query", "x", "--format", "xml"}, exitUsage},
		{"summary bad format", []string{"summary", "--query", "x", "--format", "ndjson"}, exitUsage},
		{"patterns bad format", []string{"patterns", "--query", "x", "--format", "csv"}, exitUsage},
	}
	for _, tt := range withServer {
		t.Run(tt.name, func(t *testing.T) {
			setupEnv(t, srv.URL)
			_, code := runExecuteCapture(t, tt.args...)
			assert.Equal(t, tt.want, code, "args=%v", tt.args)
		})
	}

	// missingAuth cases: a real runtime failure (exit 1) when the
	// command would otherwise reach the API.
	missingAuth := []struct {
		name string
		args []string
	}{
		{"fetch missing auth", []string{"fetch", "--query", "service:web"}},
		{"summary missing auth", []string{"summary", "--query", "service:web"}},
		{"patterns missing auth", []string{"patterns", "--query", "service:web"}},
	}
	for _, tt := range missingAuth {
		t.Run(tt.name, func(t *testing.T) {
			clearAuth(t)
			out, code := runExecuteCapture(t, tt.args...)
			assert.Equal(t, exitError, code, "args=%v", tt.args)
			assert.Contains(t, out, "auth_missing")
		})
	}
}

func TestExitCodeAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"errors":["Forbidden"]}`)
	}))
	defer srv.Close()

	for _, sub := range []string{"fetch", "summary", "patterns"} {
		t.Run(sub+" 401", func(t *testing.T) {
			setupEnv(t, srv.URL)
			out, code := runExecuteCapture(t, sub, "--query", "service:web")
			assert.Equal(t, exitError, code)
			assert.Contains(t, out, "auth_failed")
		})
	}
}

// TestVersionStdout confirms `version` actually prints to stdout (the
// matrix only checks its exit code).
func TestVersionStdout(t *testing.T) {
	out, code := runExecuteCapture(t, "version")
	assert.Equal(t, exitOK, code)
	assert.True(t, strings.HasPrefix(out, "dogfetch "), "version output: %q", out)
}

func TestImplicitFetchDispatch(t *testing.T) {
	// Leading dash dispatches to fetch without the explicit verb.
	clearAuth(t)
	_, code := runExecuteCapture(t, "--query")
	// "--query" with no value is a parse error (usage), proving the
	// implicit-fetch shim routed it to the fetch flag set.
	assert.Equal(t, exitUsage, code)
}
