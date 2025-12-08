package writer

import (
	"fmt"
	"io"
	"os"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
)

// Writer defines the interface for writing log data
type Writer interface {
	// WritePage writes a page of logs
	WritePage(logs []datadogV2.Log) error

	// Finalize is called after all pages have been written
	Finalize() error

	// Close releases any resources
	Close() error
}

// New creates a new writer based on format
// If path is empty, writes to stdout
func New(format, path string, append bool) (Writer, error) {
	switch format {
	case "json":
		if path == "" {
			return NewJSONWriterWithOutput(os.Stdout)
		}
		return NewJSONWriter(path)
	case "ndjson":
		if path == "" {
			return NewNDJSONWriterWithOutput(os.Stdout)
		}
		return NewNDJSONWriter(path, append)
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}

// getWriter returns an io.WriteCloser for the given path
// Returns stdout if path is empty
func getWriter(path string, append bool) (io.WriteCloser, error) {
	if path == "" {
		return os.Stdout, nil
	}

	flags := os.O_CREATE | os.O_WRONLY
	if append {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}

	return os.OpenFile(path, flags, 0644)
}
