package writer

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNDJSONWriterWithOutput(t *testing.T) {
	var buf bytes.Buffer
	w, err := NewNDJSONWriterWithOutput(&buf)
	require.NoError(t, err)

	// Create test logs
	logs := createTestLogs(3)

	// Write logs
	require.NoError(t, w.WritePage(logs))
	require.NoError(t, w.Finalize())
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

func TestNDJSONWriterWithFile(t *testing.T) {
	tmpfile := createTempFile(t)
	defer os.Remove(tmpfile)

	w, err := NewNDJSONWriter(tmpfile, false)
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
	w1, err := NewNDJSONWriter(tmpfile, false)
	require.NoError(t, err)
	require.NoError(t, w1.WritePage(createTestLogs(2)))
	require.NoError(t, w1.Close())

	// Append second batch
	w2, err := NewNDJSONWriter(tmpfile, true)
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
	w, err := NewJSONWriterWithOutput(&buf)
	require.NoError(t, err)

	// Write multiple pages
	require.NoError(t, w.WritePage(createTestLogs(2)))
	require.NoError(t, w.WritePage(createTestLogs(3)))
	require.NoError(t, w.Finalize())

	// Verify output structure
	var output map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &output))

	logs, ok := output["logs"].([]interface{})
	require.True(t, ok, "Output should have 'logs' array")
	assert.Len(t, logs, 5)

	meta, ok := output["meta"].(map[string]interface{})
	require.True(t, ok, "Output should have 'meta' object")
	assert.Equal(t, float64(5), meta["total_fetched"])
	assert.Equal(t, float64(2), meta["pages"])
}

func TestJSONWriterWithFile(t *testing.T) {
	tmpfile := createTempFile(t)
	defer os.Remove(tmpfile)

	w, err := NewJSONWriter(tmpfile)
	require.NoError(t, err)

	require.NoError(t, w.WritePage(createTestLogs(3)))
	require.NoError(t, w.Finalize())

	// Read and verify file
	content, err := os.ReadFile(tmpfile)
	require.NoError(t, err)

	var output map[string]interface{}
	require.NoError(t, json.Unmarshal(content, &output))

	logs, ok := output["logs"].([]interface{})
	require.True(t, ok)
	assert.Len(t, logs, 3)
}

func TestNewWriter(t *testing.T) {
	tests := []struct {
		name    string
		format  string
		path    string
		append  bool
		wantErr bool
	}{
		{
			name:    "json to file",
			format:  "json",
			path:    createTempFile(t),
			append:  false,
			wantErr: false,
		},
		{
			name:    "ndjson to file",
			format:  "ndjson",
			path:    createTempFile(t),
			append:  false,
			wantErr: false,
		},
		{
			name:    "json to stdout",
			format:  "json",
			path:    "",
			append:  false,
			wantErr: false,
		},
		{
			name:    "ndjson to stdout",
			format:  "ndjson",
			path:    "",
			append:  false,
			wantErr: false,
		},
		{
			name:    "invalid format",
			format:  "xml",
			path:    "",
			append:  false,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.path != "" {
				defer os.Remove(tt.path)
			}

			w, err := New(tt.format, tt.path, tt.append)

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
	for i := 0; i < count; i++ {
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
