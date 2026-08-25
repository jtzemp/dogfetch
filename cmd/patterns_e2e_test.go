//go:build testseam

package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// repetitiveLogs builds a logs page: 5 payment failures, 3 timeouts,
// 1 odd one out.
func repetitiveLogs() string {
	var logs []map[string]any
	add := func(i int, msg string) {
		logs = append(logs, map[string]any{
			"id": fmt.Sprintf("log-%d", len(logs)),
			"attributes": map[string]any{
				"timestamp": fmt.Sprintf("2026-06-11T10:00:0%dZ", i%10),
				"status":    "error",
				"service":   "web",
				"message":   msg,
			},
		})
	}
	for i := range 5 {
		add(i, fmt.Sprintf("failed to process payment %d for user u%d: card_declined", i, i))
	}
	for i := range 3 {
		add(i, fmt.Sprintf("connection to 10.0.0.%d:5432 timed out after 30s", i))
	}
	add(9, "schema migration completed successfully")
	b, _ := json.Marshal(map[string]any{"data": logs})
	return string(b)
}

func runPatternsCapture(t *testing.T, args ...string) (string, int) {
	t.Helper()
	return captureStdout(t, func() int { return runPatterns(args) })
}

func TestE2EPatternsToon(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, repetitiveLogs())
	}))
	defer srv.Close()
	setupEnv(t, srv.URL)

	out, code := runPatternsCapture(t, "--query", "service:web status:error")
	assert.Equal(t, 0, code)

	assert.Contains(t, out, "scanned: 9")
	assert.Contains(t, out, "patterns[3]{count,first_seen,last_seen,pattern}:")
	assert.Contains(t, out, "5,2026-06-11T10:00:00Z,2026-06-11T10:00:04Z,failed to process payment <*> for user <*> card_declined")
	assert.Contains(t, out, "3,2026-06-11T10:00:00Z,2026-06-11T10:00:02Z,connection to <*> timed out after <*>")
	assert.Contains(t, out, "1,")
	assert.Contains(t, out, "schema migration completed successfully")
	assert.Contains(t, out, "Drill into one pattern: dogfetch fetch --query 'service:web status:error \"<literal text from pattern>\"' --limit 20")
	assert.Contains(t, out, "Add --samples to include one raw example per pattern")
}

func TestE2EPatternsSamplesAndTop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, repetitiveLogs())
	}))
	defer srv.Close()
	setupEnv(t, srv.URL)

	out, code := runPatternsCapture(t, "--query", "service:web", "--samples", "--top", "1")
	assert.Equal(t, 0, code)
	assert.Contains(t, out, "patterns[1]{count,first_seen,last_seen,pattern,sample}:")
	assert.Contains(t, out, "failed to process payment 0 for user u0: card_declined", "sample is the raw message")
	assert.Contains(t, out, "Showing top 1 of 3 patterns; rerun with --top 0 for all")
	assert.NotContains(t, out, "Add --samples")
}

func TestE2EPatternsJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, repetitiveLogs())
	}))
	defer srv.Close()
	setupEnv(t, srv.URL)

	out, code := runPatternsCapture(t, "--query", "service:web", "--format", "json")
	assert.Equal(t, 0, code)

	var obj struct {
		Scanned       int          `json:"scanned"`
		TotalPatterns int          `json:"total_patterns"`
		Patterns      []patternRow `json:"patterns"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &obj))
	assert.Equal(t, 9, obj.Scanned)
	assert.Equal(t, 3, obj.TotalPatterns)
	require.Len(t, obj.Patterns, 3)
	assert.Equal(t, int64(5), obj.Patterns[0].Count)
}

func TestE2EPatternsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data": []}`)
	}))
	defer srv.Close()
	setupEnv(t, srv.URL)

	out, code := runPatternsCapture(t,
		"--query", "service:nope",
		"--from", "2026-06-10T00:00:00Z", "--to", "2026-06-11T00:00:00Z")
	assert.Equal(t, 0, code)
	want := "patterns: 0 logs matched query 'service:nope' in range 2026-06-10T00:00:00Z to 2026-06-11T00:00:00Z\n" +
		"help[1]:\n" +
		"  Widen the time range with --from 24h or loosen the query\n"
	assert.Equal(t, want, out)
}

func TestE2EPatternsScanCapHint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Always offer another page so the --limit cap is what stops us.
		var logs []map[string]any
		for i := range 5 {
			logs = append(logs, map[string]any{
				"id": fmt.Sprintf("log-%d", i),
				"attributes": map[string]any{
					"timestamp": "2026-06-11T10:00:00Z",
					"message":   fmt.Sprintf("request %d handled", i),
				},
			})
		}
		b, _ := json.Marshal(map[string]any{
			"data": logs,
			"meta": map[string]any{"page": map[string]any{"after": "more"}},
		})
		_, _ = w.Write(b)
	}))
	defer srv.Close()
	setupEnv(t, srv.URL)

	out, code := runPatternsCapture(t, "--query", "service:web", "--limit", "5")
	assert.Equal(t, 0, code)
	assert.Contains(t, out, "scanned: 5")
	assert.Contains(t, out, "Scanned the first 5 matching logs; raise --limit to scan more")
}

func TestE2EPatternsBadFormat(t *testing.T) {
	setupEnv(t, "http://127.0.0.1:0")
	out, code := runPatternsCapture(t, "--format", "csv")
	assert.Equal(t, 2, code)
	assert.Contains(t, out, "patterns format must be 'toon' or 'json'")
}
