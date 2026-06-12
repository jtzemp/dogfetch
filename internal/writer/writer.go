package writer

import (
	"fmt"
	"os"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	"github.com/jtzemp/dogfetch/internal/config"
	"github.com/jtzemp/dogfetch/internal/project"
)

// Meta summarizes a completed fetch for Finalize.
type Meta struct {
	Total      int
	Pages      int
	NextCursor string // non-empty when more logs are available
	Query      string
	From, To   time.Time
	Elapsed    time.Duration
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
