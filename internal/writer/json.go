package writer

import (
	"encoding/json"
	"io"
	"os"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	"github.com/jtzemp/dogfetch/internal/project"
)

// JSONWriter buffers all logs in memory and writes a single JSON file
type JSONWriter struct {
	path        string
	output      io.Writer
	proj        *project.Projector // nil = full log objects
	logs        []datadogV2.Log
	pageCount   int
	shouldClose bool
}

// NewJSONWriter creates a new JSON writer for a file
func NewJSONWriter(path string, proj *project.Projector) (*JSONWriter, error) {
	return &JSONWriter{
		path:        path,
		proj:        proj,
		logs:        make([]datadogV2.Log, 0),
		shouldClose: true,
	}, nil
}

// NewJSONWriterWithOutput creates a new JSON writer for any io.Writer
func NewJSONWriterWithOutput(w io.Writer, proj *project.Projector) (*JSONWriter, error) {
	return &JSONWriter{
		output:      w,
		proj:        proj,
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
func (w *JSONWriter) Finalize(meta Meta) error {
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

	var logs any = w.logs
	if w.proj != nil {
		projected := make([]map[string]string, len(w.logs))
		for i, log := range w.logs {
			projected[i] = w.proj.Map(log)
		}
		logs = projected
	}

	outMeta := map[string]any{
		"total_fetched": len(w.logs),
		"pages":         w.pageCount,
	}
	if meta.NextCursor != "" {
		outMeta["next_cursor"] = meta.NextCursor
	}

	output := map[string]any{
		"logs": logs,
		"meta": outMeta,
	}

	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

// Close is a no-op for JSONWriter
func (w *JSONWriter) Close() error {
	return nil
}
