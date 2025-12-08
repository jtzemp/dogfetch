package writer

import (
	"encoding/json"
	"io"
	"os"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
)

// NDJSONWriter streams logs to a newline-delimited JSON file
type NDJSONWriter struct {
	writer     io.Writer
	closer     io.Closer
	encoder    *json.Encoder
	shouldClose bool
}

// NewNDJSONWriter creates a new NDJSON writer for a file
func NewNDJSONWriter(path string, append bool) (*NDJSONWriter, error) {
	flags := os.O_CREATE | os.O_WRONLY
	if append {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}

	f, err := os.OpenFile(path, flags, 0644)
	if err != nil {
		return nil, err
	}

	return &NDJSONWriter{
		writer:      f,
		closer:      f,
		encoder:     json.NewEncoder(f),
		shouldClose: true,
	}, nil
}

// NewNDJSONWriterWithOutput creates a new NDJSON writer for any io.Writer
func NewNDJSONWriterWithOutput(w io.Writer) (*NDJSONWriter, error) {
	return &NDJSONWriter{
		writer:      w,
		encoder:     json.NewEncoder(w),
		shouldClose: false,
	}, nil
}

// WritePage writes logs immediately to the file (one per line)
func (w *NDJSONWriter) WritePage(logs []datadogV2.Log) error {
	for _, log := range logs {
		if err := w.encoder.Encode(log); err != nil {
			return err
		}
	}
	return nil
}

// Finalize is a no-op for NDJSONWriter (already written)
func (w *NDJSONWriter) Finalize() error {
	return nil
}

// Close closes the output file (if it's a file)
func (w *NDJSONWriter) Close() error {
	if w.shouldClose && w.closer != nil {
		return w.closer.Close()
	}
	return nil
}
