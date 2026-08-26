package writer

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	"github.com/jtzemp/dogfetch/internal/config"
	"github.com/jtzemp/dogfetch/internal/project"
)

// Meta summarizes a completed fetch for Finalize.
type Meta struct {
	Total      int
	NextCursor string // non-empty when more logs are available
	Query      string
	From, To   time.Time
}

// Writer defines the interface for writing log data
type Writer interface {
	// WritePage writes a page of logs
	WritePage(logs []datadogV2.Log) error

	// Finalize is called after all pages have been written
	Finalize(meta Meta) error

	// Close releases any resources
	Close() error
}

// New creates a writer for cfg.Format. An empty cfg.OutputPath writes
// to stdout. For json/ndjson a projector is applied only when --fields
// was given (file export stays lossless by default); toon always
// projects.
func New(cfg *config.Config) (Writer, error) {
	var proj *project.Projector
	if len(cfg.Fields) > 0 {
		proj = project.New(cfg.Fields)
	}

	switch cfg.Format {
	case "json":
		if cfg.OutputPath == "" {
			return NewJSONWriterWithOutput(os.Stdout, proj)
		}
		return NewJSONWriter(cfg.OutputPath, proj)
	case "ndjson":
		if cfg.OutputPath == "" {
			return NewNDJSONWriterWithOutput(os.Stdout, proj)
		}
		return NewNDJSONWriter(cfg.OutputPath, cfg.Append, proj)
	case "toon":
		if cfg.OutputPath == "" {
			return NewTOONWriterWithOutput(os.Stdout, project.New(cfg.Fields)), nil
		}
		return NewTOONWriter(cfg.OutputPath, project.New(cfg.Fields)), nil
	default:
		return nil, fmt.Errorf("unsupported format: %s", cfg.Format)
	}
}

// openOut resolves a writer's Finalize destination: the io.Writer it
// was constructed with, or a freshly created file at path. The
// returned close func is a no-op for the caller-supplied writer.
func openOut(output io.Writer, path string) (io.Writer, func() error, error) {
	if output != nil {
		return output, func() error { return nil }, nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, config.FileMode())
	if err != nil {
		return nil, nil, err
	}
	return f, f.Close, nil
}
