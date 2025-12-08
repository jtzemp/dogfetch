package writer

import (
	"encoding/json"
	"io"
	"os"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
)

// JSONWriter buffers all logs in memory and writes a single JSON file
type JSONWriter struct {
	path        string
	output      io.Writer
	logs        []datadogV2.Log
	pageCount   int
	shouldClose bool
	closer      io.Closer
}

// NewJSONWriter creates a new JSON writer for a file
func NewJSONWriter(path string) (*JSONWriter, error) {
	return &JSONWriter{
		path:        path,
		logs:        make([]datadogV2.Log, 0),
		shouldClose: true,
	}, nil
}

// NewJSONWriterWithOutput creates a new JSON writer for any io.Writer
func NewJSONWriterWithOutput(w io.Writer) (*JSONWriter, error) {
	return &JSONWriter{
		output:      w,
		logs:        make([]datadogV2.Log, 0),
		shouldClose: false,
	}, nil
}

// WritePage buffers the logs in memory
func (w *JSONWriter) WritePage(logs []datadogV2.Log) error {
	w.logs = append(w.logs, logs...)
	w.pageCount++
	return nil
}

// Finalize writes all buffered logs to the output
func (w *JSONWriter) Finalize() error {
	var out io.Writer

	if w.output != nil {
		// Writing to provided writer (e.g., stdout)
		out = w.output
	} else {
		// Writing to file
		f, err := os.Create(w.path)
		if err != nil {
			return err
		}
		defer f.Close()
		out = f
	}

	output := map[string]interface{}{
		"logs": w.logs,
		"meta": map[string]interface{}{
			"total_fetched": len(w.logs),
			"pages":         w.pageCount,
		},
	}

	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

// Close is a no-op for JSONWriter
func (w *JSONWriter) Close() error {
	return nil
}
