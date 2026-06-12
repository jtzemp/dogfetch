package writer

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jtzemp/dogfetch/internal/config"
	"github.com/jtzemp/dogfetch/internal/project"
)

func TestNDJSONWriterWithOutput(t *testing.T) {
	var buf bytes.Buffer
	w, err := NewNDJSONWriterWithOutput(&buf, nil)
	require.NoError(t, err)

	// Create test logs
	logs := createTestLogs(3)

	// Write logs
	require.NoError(t, w.WritePage(logs))
	require.NoError(t, w.Finalize(Meta{}))
	require.NoError(t, w.Close())

	// Verify output
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	assert.Len(t, lines, 3)

	// Verify each line is valid JSON
	for i, line := range lines {
		var log datadogV2.Log
		assert.NoError(t, json.Unmarshal([]byte(line), &log), "Line %d should be valid JSON", i)
	}
}

func TestNDJSONWriterProjected(t *testing.T) {
	var buf bytes.Buffer
	w, err := NewNDJSONWriterWithOutput(&buf, project.New([]string{"id", "message"}))
	require.NoError(t, err)

	require.NoError(t, w.WritePage(createTestLogs(2)))
	require.NoError(t, w.Finalize(Meta{}))

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.Len(t, lines, 2)

	var row map[string]string
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &row))
	assert.Equal(t, map[string]string{"id": "test-id", "message": "test message"}, row)
}

func TestNDJSONWriterWithFile(t *testing.T) {
	tmpfile := createTempFile(t)
	defer os.Remove(tmpfile)

	w, err := NewNDJSONWriter(tmpfile, false, nil)
	require.NoError(t, err)

	logs := createTestLogs(2)
	require.NoError(t, w.WritePage(logs))
	require.NoError(t, w.Close())

	// Read and verify file
	content, err := os.ReadFile(tmpfile)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	assert.Len(t, lines, 2)
}

func TestNDJSONWriterAppend(t *testing.T) {
	tmpfile := createTempFile(t)
	defer os.Remove(tmpfile)

	// Write first batch
	w1, err := NewNDJSONWriter(tmpfile, false, nil)
	require.NoError(t, err)
	require.NoError(t, w1.WritePage(createTestLogs(2)))
	require.NoError(t, w1.Close())

	// Append second batch
	w2, err := NewNDJSONWriter(tmpfile, true, nil)
	require.NoError(t, err)
	require.NoError(t, w2.WritePage(createTestLogs(3)))
	require.NoError(t, w2.Close())

	// Verify total lines
	content, err := os.ReadFile(tmpfile)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	assert.Len(t, lines, 5)
}

func TestJSONWriterWithOutput(t *testing.T) {
	var buf bytes.Buffer
	w, err := NewJSONWriterWithOutput(&buf, nil)
	require.NoError(t, err)

	// Write multiple pages
	require.NoError(t, w.WritePage(createTestLogs(2)))
	require.NoError(t, w.WritePage(createTestLogs(3)))
	require.NoError(t, w.Finalize(Meta{}))

	// Verify output structure
	var output map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &output))

	logs, ok := output["logs"].([]any)
	require.True(t, ok, "Output should have 'logs' array")
	assert.Len(t, logs, 5)

	meta, ok := output["meta"].(map[string]any)
	require.True(t, ok, "Output should have 'meta' object")
	assert.Equal(t, float64(5), meta["total_fetched"])
	assert.Equal(t, float64(2), meta["pages"])
	assert.NotContains(t, meta, "next_cursor")
}

func TestJSONWriterNextCursor(t *testing.T) {
	var buf bytes.Buffer
	w, err := NewJSONWriterWithOutput(&buf, nil)
	require.NoError(t, err)
	require.NoError(t, w.WritePage(createTestLogs(1)))
	require.NoError(t, w.Finalize(Meta{NextCursor: "abc123"}))

	var output map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &output))
	meta := output["meta"].(map[string]any)
	assert.Equal(t, "abc123", meta["next_cursor"])
}

func TestJSONWriterWithFile(t *testing.T) {
	tmpfile := createTempFile(t)
	defer os.Remove(tmpfile)

	w, err := NewJSONWriter(tmpfile, nil)
	require.NoError(t, err)

	require.NoError(t, w.WritePage(createTestLogs(3)))
	require.NoError(t, w.Finalize(Meta{}))

	// Read and verify file
	content, err := os.ReadFile(tmpfile)
	require.NoError(t, err)

	var output map[string]any
	require.NoError(t, json.Unmarshal(content, &output))

	logs, ok := output["logs"].([]any)
	require.True(t, ok)
	assert.Len(t, logs, 3)
}

func TestTOONWriterOutput(t *testing.T) {
	var buf bytes.Buffer
	w := NewTOONWriterWithOutput(&buf, project.New(nil))

	require.NoError(t, w.WritePage(createTestLogs(2)))
	require.NoError(t, w.Finalize(Meta{Total: 2, Query: "service:web"}))

	out := buf.String()
	assert.Contains(t, out, "count: 2")
	assert.Contains(t, out, "logs[2]{timestamp,status,service,message}:")
	assert.Contains(t, out, "help[")
	assert.NotContains(t, out, "--cursor", "no cursor hint without NextCursor")
}

func TestTOONWriterCursorHint(t *testing.T) {
	var buf bytes.Buffer
	w := NewTOONWriterWithOutput(&buf, project.New(nil))
	require.NoError(t, w.WritePage(createTestLogs(1)))
	require.NoError(t, w.Finalize(Meta{Total: 1, NextCursor: "tok42"}))
	assert.Contains(t, buf.String(), "--cursor 'tok42'")
}

func TestTOONWriterEmptyState(t *testing.T) {
	var buf bytes.Buffer
	w := NewTOONWriterWithOutput(&buf, project.New(nil))
	from := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	require.NoError(t, w.Finalize(Meta{Query: "service:web", From: from}))

	out := buf.String()
	assert.Contains(t, out, "logs: 0 matched query 'service:web' in range 2026-06-10T00:00:00Z to now")
}

func TestNewWriter(t *testing.T) {
	tests := []struct {
		name    string
		format  string
		path    string
		wantErr bool
	}{
		{name: "json to file", format: "json", path: createTempFile(t)},
		{name: "ndjson to file", format: "ndjson", path: createTempFile(t)},
		{name: "toon to file", format: "toon", path: createTempFile(t)},
		{name: "json to stdout", format: "json"},
		{name: "ndjson to stdout", format: "ndjson"},
		{name: "toon to stdout", format: "toon"},
		{name: "invalid format", format: "xml", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.path != "" {
				defer os.Remove(tt.path)
			}

			w, err := New(&config.Config{Format: tt.format, OutputPath: tt.path})

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, w)
				defer w.Close()
			}
		})
	}
}

// Helper functions

func createTestLogs(count int) []datadogV2.Log {
	logs := make([]datadogV2.Log, count)
	for i := range count {
		id := "test-id"
		message := "test message"
		logs[i] = datadogV2.Log{
			Id: &id,
			Attributes: &datadogV2.LogAttributes{
				Message: &message,
			},
		}
	}
	return logs
}

func createTempFile(t *testing.T) string {
	t.Helper()
	f, err := os.CreateTemp("", "dogfetch-test-*.json")
	require.NoError(t, err)
	path := f.Name()
	f.Close()
	return path
}
