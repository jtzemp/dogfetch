package writer

import (
	"encoding/json"
	"os"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
)

// NDJSONWriter streams logs to a newline-delimited JSON file
type NDJSONWriter struct {
	file    *os.File
	encoder *json.Encoder
}

// NewNDJSONWriter creates a new NDJSON writer
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
		file:    f,
		encoder: json.NewEncoder(f),
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

// Close closes the output file
func (w *NDJSONWriter) Close() error {
	if w.file != nil {
		return w.file.Close()
	}
	return nil
}
