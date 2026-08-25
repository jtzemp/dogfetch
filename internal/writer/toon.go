package writer

import (
	"fmt"
	"io"
	"strings"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	"github.com/jtzemp/dogfetch/internal/project"
	"github.com/jtzemp/dogfetch/internal/toon"
)

// TOONWriter buffers projected rows and emits a TOON document on
// Finalize. The buffer is bounded in practice: TOON targets agent
// context windows, so it pairs with --limit rather than full exports.
type TOONWriter struct {
	path   string
	output io.Writer
	proj   *project.Projector
	rows   [][]string
}

// NewTOONWriterWithOutput creates a TOON writer for any io.Writer.
func NewTOONWriterWithOutput(w io.Writer, proj *project.Projector) *TOONWriter {
	return &TOONWriter{output: w, proj: proj}
}

// NewTOONWriter creates a TOON writer for a file.
func NewTOONWriter(path string, proj *project.Projector) *TOONWriter {
	return &TOONWriter{path: path, proj: proj}
}

// WritePage projects and buffers the logs.
func (w *TOONWriter) WritePage(logs []datadogV2.Log) error {
	if w.rows == nil {
		w.rows = make([][]string, 0, len(logs))
	}
	for _, log := range logs {
		w.rows = append(w.rows, w.proj.Row(log))
	}
	return nil
}

// Finalize emits the TOON document: count, the logs table (or a
// definitive empty state), and a trailing help block with next steps.
func (w *TOONWriter) Finalize(meta Meta) error {
	out, closeOut, err := openOut(w.output, w.path)
	if err != nil {
		return err
	}
	defer closeOut() //nolint:errcheck // the encoder's error is the one that matters

	enc := toon.NewEncoder(out)

	if len(w.rows) == 0 {
		toon.EmptyState(enc, "logs", "", meta.Query, meta.From, meta.To)
		return enc.Err()
	}

	enc.Scalar("count", len(w.rows))
	enc.Table("logs", w.proj.Fields, toon.StringRows(w.rows))

	var help []string
	if w.proj.Truncated {
		help = append(help, fmt.Sprintf(
			"Values over %d chars are truncated; use --output <file> --format ndjson for full content",
			project.MaxValueLen))
	}
	help = append(help, fmt.Sprintf(
		"Add fields with --fields %s,host (any Datadog attribute path works, e.g. http.status_code)",
		strings.Join(project.DefaultFields, ",")))
	if meta.NextCursor != "" {
		help = append(help, fmt.Sprintf(
			"More logs match; fetch the next page with --cursor '%s'", meta.NextCursor))
	}
	enc.List("help", help)
	return enc.Err()
}

// Close is a no-op for TOONWriter.
func (w *TOONWriter) Close() error {
	return nil
}
