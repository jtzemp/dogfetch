// Package toon implements the subset of TOON (Token-Oriented Object
// Notation) that dogfetch emits: scalar key/value lines, tabular array
// blocks, and AXI-style help lists.
//
// It targets the TOON Working Draft v3.2 (https://toonformat.dev). A
// full encoder is deliberately avoided: the spec is a moving target and
// our output never nests, so ~150 lines with golden tests beats a
// dependency.
package toon

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

const indent = "  "

// Encoder writes TOON blocks to an underlying writer. Output is
// buffered: a table is one line per row, and an unbuffered os.Stdout
// would turn each into its own write syscall.
type Encoder struct {
	w   *bufio.Writer
	err error
}

// NewEncoder returns an Encoder writing to w. Because output is
// buffered, every caller must finish with Err: it flushes and reports
// the first write error. Output written but never flushed is lost.
func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{w: bufio.NewWriter(w)}
}

// Err flushes any buffered output and returns the first write error
// encountered, if any.
func (e *Encoder) Err() error {
	if flushErr := e.w.Flush(); flushErr != nil && e.err == nil {
		e.err = flushErr
	}
	return e.err
}

func (e *Encoder) printf(format string, args ...any) {
	if e.err != nil {
		return
	}
	_, e.err = fmt.Fprintf(e.w, format, args...)
}

// Scalar writes a single `key: value` line.
func (e *Encoder) Scalar(key string, value any) {
	switch v := value.(type) {
	case string:
		e.printf("%s: %s\n", key, Quote(v))
	default:
		e.printf("%s: %v\n", key, v)
	}
}

// Table writes a tabular array block:
//
//	name[N]{f1,f2}:
//	  v1,v2
//
// Each row must have len(fields) cells. String cells are quoted as
// needed; numeric cells render bare.
func (e *Encoder) Table(name string, fields []string, rows [][]any) {
	e.printf("%s[%d]{%s}:\n", name, len(rows), strings.Join(fields, ","))
	for _, row := range rows {
		cells := make([]string, len(row))
		for i, v := range row {
			if s, ok := v.(string); ok {
				cells[i] = Quote(s)
			} else {
				cells[i] = fmt.Sprintf("%v", v)
			}
		}
		e.printf("%s%s\n", indent, strings.Join(cells, ","))
	}
}

// StringRows converts projected string rows to Table cells.
func StringRows(rows [][]string) [][]any {
	out := make([][]any, len(rows))
	for i, row := range rows {
		cells := make([]any, len(row))
		for j, v := range row {
			cells[j] = v
		}
		out[i] = cells
	}
	return out
}

// List writes an AXI-style help block: a `name[N]:` header followed by
// one indented line per item. Items are prose and command templates
// rather than data cells, so they are not quoted the way a Scalar or a
// Table cell is - wrapping them would break the bare block format
// agents are told to read. They are still passed through Line, because
// one newline inside an item ends the line early and the rest of it
// lands in the document as forged top-level keys.
func (e *Encoder) List(name string, items []string) {
	if len(items) == 0 {
		return
	}
	e.printf("%s[%d]:\n", name, len(items))
	for _, item := range items {
		e.printf("%s%s\n", indent, Line(item))
	}
}

// Line renders s as a single output line: every character that would
// end the line or steer a terminal collapses to a space. It is the
// last-resort guard on List; callers that interpolate a value into a
// help line should use HelpArg, which also handles the shell quoting.
func Line(s string) string {
	if !strings.ContainsFunc(s, unsafeRune) {
		return s
	}
	return strings.Map(func(r rune) rune {
		if unsafeRune(r) {
			return ' '
		}
		return r
	}, s)
}

// HelpArg renders s for use inside a single-quoted argument of a
// command in a help[] block, e.g. --query '<here>'. Such a command is
// meant to be runnable, and an agent following AXI output is the one
// running it, so an interpolated value has to survive two layers: it
// must not end the TOON line (see Line), and it must not close the
// shell quote. Single quotes are escaped the only way POSIX allows:
// close, backslash-escape, reopen.
func HelpArg(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case unsafeRune(r):
			b.WriteByte(' ')
		case r == '\'':
			b.WriteString(`'\''`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// unsafeRune reports whether r must never reach the output raw. C0 and
// C1 controls cover the line-ending characters and ESC, which a
// terminal acts on rather than prints - a log message carrying ESC[2K
// can erase output a human is reading as a result. The bidi overrides
// reorder how a line renders without changing the bytes an agent
// parses, so the two readers disagree about what the line says.
func unsafeRune(r rune) bool {
	return r < 0x20 || r == 0x7f ||
		(r >= 0x80 && r <= 0x9f) ||
		(r >= 0x202a && r <= 0x202e) ||
		(r >= 0x2066 && r <= 0x2069)
}

// numberLike matches strings that would parse as a TOON number.
var numberLike = regexp.MustCompile(`^-?(0|[1-9]\d*)(\.\d+)?([eE][+-]?\d+)?$`)

// Quote returns s quoted/escaped per TOON rules when necessary, or s
// unchanged when it is safe as a bare string.
func Quote(s string) string {
	if needsQuoting(s) {
		var b strings.Builder
		b.Grow(len(s) + 2)
		b.WriteByte('"')
		for _, r := range s {
			switch r {
			case '"':
				b.WriteString(`\"`)
			case '\\':
				b.WriteString(`\\`)
			case '\n':
				b.WriteString(`\n`)
			case '\r':
				b.WriteString(`\r`)
			case '\t':
				b.WriteString(`\t`)
			default:
				if unsafeRune(r) {
					fmt.Fprintf(&b, `\u%04x`, r)
					continue
				}
				b.WriteRune(r)
			}
		}
		b.WriteByte('"')
		return b.String()
	}
	return s
}

func needsQuoting(s string) bool {
	if s == "" {
		return true
	}
	if s != strings.TrimSpace(s) {
		return true
	}
	switch s {
	case "true", "false", "null":
		return true
	}
	// Only a sign or a digit can start a number; skipping the regexp
	// otherwise keeps log messages off the engine entirely.
	if c := s[0]; (c == '-' || (c >= '0' && c <= '9')) && numberLike.MatchString(s) {
		return true
	}
	switch s[0] {
	case '-', '"', '#', '[', ']', '{', '}':
		return true
	}
	// A bare colon is safe (timestamps, URLs); only "key: value"
	// ambiguity needs quoting.
	if strings.Contains(s, ": ") || strings.HasSuffix(s, ":") {
		return true
	}
	if strings.ContainsAny(s, ",\"\n\r\t\\") {
		return true
	}
	// ESC and friends are not in that set but must still be escaped
	// rather than handed to whatever renders this line.
	return strings.ContainsFunc(s, unsafeRune)
}

// FormatRangeTime renders a query range bound. A zero time means the
// bound was never set, which for a range end reads as "now".
func FormatRangeTime(t time.Time) string {
	if t.IsZero() {
		return "now"
	}
	return t.UTC().Format(time.RFC3339)
}

// EmptyState writes the shared AXI no-results block: a definitive
// "0 <noun> matched ..." answer under key, plus the standard hint. An
// empty noun renders the bare "0 matched query ..." form.
func EmptyState(e *Encoder, key, noun, query string, from, to time.Time) {
	subject := "0"
	if noun != "" {
		subject += " " + noun
	}
	e.Scalar(key, fmt.Sprintf("%s matched query '%s' in range %s to %s",
		subject, query, FormatRangeTime(from), FormatRangeTime(to)))
	e.List("help", []string{
		"Widen the time range with --from 24h or loosen the query",
	})
}
