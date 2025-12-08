package writer

import (
	"encoding/json"
	"os"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
)

// JSONWriter buffers all logs in memory and writes a single JSON file
type JSONWriter struct {
	path      string
	logs      []datadogV2.Log
	pageCount int
}

// NewJSONWriter creates a new JSON writer
func NewJSONWriter(path string) (*JSONWriter, error) {
	return &JSONWriter{
		path: path,
		logs: make([]datadogV2.Log, 0),
	}, nil
}

// WritePage buffers the logs in memory
func (w *JSONWriter) WritePage(logs []datadogV2.Log) error {
	w.logs = append(w.logs, logs...)
	w.pageCount++
	return nil
}

// Finalize writes all buffered logs to the output file
func (w *JSONWriter) Finalize() error {
	f, err := os.Create(w.path)
	if err != nil {
		return err
	}
	defer f.Close()

	output := map[string]interface{}{
		"logs": w.logs,
		"meta": map[string]interface{}{
			"total_fetched": len(w.logs),
			"pages":         w.pageCount,
		},
	}

	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

// Close is a no-op for JSONWriter
func (w *JSONWriter) Close() error {
	return nil
}
