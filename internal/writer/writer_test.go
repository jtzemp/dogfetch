package writer

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
)

func TestNDJSONWriterWithOutput(t *testing.T) {
	var buf bytes.Buffer
	w, err := NewNDJSONWriterWithOutput(&buf)
	if err != nil {
		t.Fatalf("NewNDJSONWriterWithOutput() error = %v", err)
	}

	// Create test logs
	logs := createTestLogs(3)

	// Write logs
	if err := w.WritePage(logs); err != nil {
		t.Fatalf("WritePage() error = %v", err)
	}

	if err := w.Finalize(); err != nil {
		t.Fatalf("Finalize() error = %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// Verify output
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 {
		t.Errorf("Expected 3 lines, got %d", len(lines))
	}

	// Verify each line is valid JSON
	for i, line := range lines {
		var log datadogV2.Log
		if err := json.Unmarshal([]byte(line), &log); err != nil {
			t.Errorf("Line %d is not valid JSON: %v", i, err)
		}
	}
}

func TestNDJSONWriterWithFile(t *testing.T) {
	tmpfile := createTempFile(t)
	defer os.Remove(tmpfile)

	w, err := NewNDJSONWriter(tmpfile, false)
	if err != nil {
		t.Fatalf("NewNDJSONWriter() error = %v", err)
	}

	logs := createTestLogs(2)
	if err := w.WritePage(logs); err != nil {
		t.Fatalf("WritePage() error = %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// Read and verify file
	content, err := os.ReadFile(tmpfile)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) != 2 {
		t.Errorf("Expected 2 lines in file, got %d", len(lines))
	}
}

func TestNDJSONWriterAppend(t *testing.T) {
	tmpfile := createTempFile(t)
	defer os.Remove(tmpfile)

	// Write first batch
	w1, err := NewNDJSONWriter(tmpfile, false)
	if err != nil {
		t.Fatalf("NewNDJSONWriter() error = %v", err)
	}
	if err := w1.WritePage(createTestLogs(2)); err != nil {
		t.Fatalf("WritePage() error = %v", err)
	}
	if err := w1.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// Append second batch
	w2, err := NewNDJSONWriter(tmpfile, true)
	if err != nil {
		t.Fatalf("NewNDJSONWriter() error = %v", err)
	}
	if err := w2.WritePage(createTestLogs(3)); err != nil {
		t.Fatalf("WritePage() error = %v", err)
	}
	if err := w2.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// Verify total lines
	content, err := os.ReadFile(tmpfile)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) != 5 {
		t.Errorf("Expected 5 lines total, got %d", len(lines))
	}
}

func TestJSONWriterWithOutput(t *testing.T) {
	var buf bytes.Buffer
	w, err := NewJSONWriterWithOutput(&buf)
	if err != nil {
		t.Fatalf("NewJSONWriterWithOutput() error = %v", err)
	}

	// Write multiple pages
	if err := w.WritePage(createTestLogs(2)); err != nil {
		t.Fatalf("WritePage() error = %v", err)
	}
	if err := w.WritePage(createTestLogs(3)); err != nil {
		t.Fatalf("WritePage() error = %v", err)
	}

	if err := w.Finalize(); err != nil {
		t.Fatalf("Finalize() error = %v", err)
	}

	// Verify output structure
	var output map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &output); err != nil {
		t.Fatalf("Output is not valid JSON: %v", err)
	}

	logs, ok := output["logs"].([]interface{})
	if !ok {
		t.Fatalf("Output missing 'logs' array")
	}

	if len(logs) != 5 {
		t.Errorf("Expected 5 logs, got %d", len(logs))
	}

	meta, ok := output["meta"].(map[string]interface{})
	if !ok {
		t.Fatalf("Output missing 'meta' object")
	}

	if totalFetched := meta["total_fetched"]; totalFetched != float64(5) {
		t.Errorf("Expected total_fetched=5, got %v", totalFetched)
	}

	if pages := meta["pages"]; pages != float64(2) {
		t.Errorf("Expected pages=2, got %v", pages)
	}
}

func TestJSONWriterWithFile(t *testing.T) {
	tmpfile := createTempFile(t)
	defer os.Remove(tmpfile)

	w, err := NewJSONWriter(tmpfile)
	if err != nil {
		t.Fatalf("NewJSONWriter() error = %v", err)
	}

	if err := w.WritePage(createTestLogs(3)); err != nil {
		t.Fatalf("WritePage() error = %v", err)
	}

	if err := w.Finalize(); err != nil {
		t.Fatalf("Finalize() error = %v", err)
	}

	// Read and verify file
	content, err := os.ReadFile(tmpfile)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var output map[string]interface{}
	if err := json.Unmarshal(content, &output); err != nil {
		t.Fatalf("File content is not valid JSON: %v", err)
	}

	logs, ok := output["logs"].([]interface{})
	if !ok || len(logs) != 3 {
		t.Errorf("Expected 3 logs in output")
	}
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
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err == nil {
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
	if err != nil {
		t.Fatalf("CreateTemp() error = %v", err)
	}
	path := f.Name()
	f.Close()
	return path
}
