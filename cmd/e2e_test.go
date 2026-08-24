package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockLogs builds a Datadog /api/v2/logs/events response body.
func mockLogs(n int, afterCursor string) string {
	logs := make([]map[string]any, n)
	for i := range n {
		logs[i] = map[string]any{
			"id": fmt.Sprintf("log-%d", i),
			"attributes": map[string]any{
				"timestamp": fmt.Sprintf("2026-06-11T10:00:0%dZ", i),
				"status":    "error",
				"service":   "web",
				"message":   fmt.Sprintf("boom %d", i),
			},
		}
	}
	body := map[string]any{"data": logs}
	if afterCursor != "" {
		body["meta"] = map[string]any{"page": map[string]any{"after": afterCursor}}
	}
	b, _ := json.Marshal(body)
	return string(b)
}

// captureStdout runs run with os.Stdout redirected, returning what it
// wrote alongside its exit code. Shared by every e2e file in the
// package; the drain goroutine keeps a large payload from deadlocking
// on the pipe buffer.
func captureStdout(t *testing.T, run func() int) (string, int) {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	outCh := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		outCh <- string(b)
	}()

	code := run()
	w.Close()
	return <-outCh, code
}

// runFetchCapture runs runFetch with stdout captured.
func runFetchCapture(t *testing.T, args ...string) (string, int) {
	t.Helper()
	return captureStdout(t, func() int { return runFetch(args) })
}

// setupEnv points the CLI at a mock server and clears DOGFETCH_* so a
// developer's environment can't leak into assertions. The mock URL goes
// through DOGFETCH_API_URL (the internal test seam), not DD_SITE, which
// only accepts real Datadog domains.
func setupEnv(t *testing.T, serverURL string) {
	t.Helper()
	t.Setenv("DD_API_KEY", "test-api-key")
	t.Setenv("DD_APP_KEY", "test-app-key")
	t.Setenv("DD_SITE", "")
	t.Setenv("DOGFETCH_API_URL", serverURL)
	for _, v := range []string{"DOGFETCH_FORMAT", "DOGFETCH_FIELDS", "DOGFETCH_LIMIT", "DOGFETCH_PAGESIZE", "DOGFETCH_INDEX"} {
		t.Setenv(v, "")
	}
}

func TestE2EToonResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, mockLogs(2, ""))
	}))
	defer srv.Close()
	setupEnv(t, srv.URL)

	out, code := runFetchCapture(t, "--query", "service:web status:error")
	assert.Equal(t, 0, code)

	want := "count: 2\n" +
		"logs[2]{timestamp,status,service,message}:\n" +
		"  2026-06-11T10:00:00Z,error,web,boom 0\n" +
		"  2026-06-11T10:00:01Z,error,web,boom 1\n" +
		"help[1]:\n" +
		"  Add fields with --fields timestamp,status,service,message,host (any Datadog attribute path works, e.g. http.status_code)\n"
	assert.Equal(t, want, out)
}

func TestE2EToonEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"data": []}`)
	}))
	defer srv.Close()
	setupEnv(t, srv.URL)

	out, code := runFetchCapture(t,
		"--query", "service:nope",
		"--from", "2026-06-10T00:00:00Z", "--to", "2026-06-11T00:00:00Z")
	assert.Equal(t, 0, code)

	want := "logs: 0 matched query 'service:nope' in range 2026-06-10T00:00:00Z to 2026-06-11T00:00:00Z\n" +
		"help[1]:\n" +
		"  Widen the time range with --from 24h or loosen the query\n"
	assert.Equal(t, want, out)
}

func TestE2EToonLimitWithCursor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, mockLogs(3, "cursor-abc"))
	}))
	defer srv.Close()
	setupEnv(t, srv.URL)

	out, code := runFetchCapture(t, "--query", "service:web", "--limit", "2")
	assert.Equal(t, 0, code)
	assert.Contains(t, out, "count: 2")
	assert.Contains(t, out, "logs[2]{timestamp,status,service,message}:")
	assert.Contains(t, out, "More logs match; fetch the next page with --cursor 'cursor-abc'")
	assert.NotContains(t, out, "boom 2", "limit should trim the page")
}

func TestE2EUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprint(w, `{"errors": ["Authentication failed"]}`)
	}))
	defer srv.Close()
	setupEnv(t, srv.URL)

	out, code := runFetchCapture(t, "--query", "service:web")
	assert.Equal(t, 1, code)
	assert.Contains(t, out, "error: \"authentication failed: Datadog rejected DD_API_KEY/DD_APP_KEY (HTTP 401) (auth_failed)\"")
	assert.Contains(t, out, "https://app.datadoghq.com/organization-settings/api-keys")
}

func TestE2EMissingQueryUsageError(t *testing.T) {
	setupEnv(t, "http://127.0.0.1:0")

	out, code := runFetchCapture(t)
	assert.Equal(t, 2, code)
	assert.Contains(t, out, "error: query is required (usage)")
	assert.Contains(t, out, "help[1]:")
}

func TestE2EMissingAuth(t *testing.T) {
	setupEnv(t, "http://127.0.0.1:0")
	t.Setenv("DD_API_KEY", "")
	t.Setenv("DD_APP_KEY", "")
	t.Setenv("HOME", t.TempDir()) // hide any real ~/.config/dogfetch/env

	out, code := runFetchCapture(t, "--query", "service:web")
	assert.Equal(t, 1, code)
	assert.Contains(t, out, "(auth_missing)")
	assert.Contains(t, out, "organization-settings/api-keys")
}

func TestE2EErrorRendersJSONWhenRequested(t *testing.T) {
	setupEnv(t, "http://127.0.0.1:0")

	out, code := runFetchCapture(t, "--format", "json")
	assert.Equal(t, 2, code)
	var obj map[string]map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &obj))
	assert.Equal(t, "usage", obj["error"]["code"])
}

func TestE2EDefaultNdjsonWithOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, mockLogs(2, ""))
	}))
	defer srv.Close()
	setupEnv(t, srv.URL)

	outFile := t.TempDir() + "/logs.ndjson"
	out, code := runFetchCapture(t, "--query", "service:web", "--output", outFile)
	assert.Equal(t, 0, code)
	assert.Empty(t, out, "file export should leave stdout empty")

	content, err := os.ReadFile(outFile)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	require.Len(t, lines, 2)
	var log map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &log))
	assert.Equal(t, "log-0", log["id"], "file export stays lossless full objects")
}

func TestE2ELegacyImplicitFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, mockLogs(1, ""))
	}))
	defer srv.Close()
	setupEnv(t, srv.URL)

	// `dogfetch --query ... --format ndjson` (pre-subcommand style)
	os.Args = []string{"dogfetch", "--query", "service:web", "--format", "ndjson"}
	r, w, err := os.Pipe()
	require.NoError(t, err)
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()
	outCh := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		outCh <- string(b)
	}()
	code := Execute()
	w.Close()
	out := <-outCh

	assert.Equal(t, 0, code)
	var log map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &log))
	assert.Equal(t, "log-0", log["id"])
}
