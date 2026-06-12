// Package project selects a small set of fields from Datadog log
// objects so agent-facing output carries only what the agent asked
// for. Raw Log blobs are huge; the default projection is the smallest
// schema that lets an agent decide its next step.
package project

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
)

// DefaultFields is the projection used when --fields is not given.
var DefaultFields = []string{"timestamp", "status", "service", "message"}

// MaxValueLen is the per-value truncation limit. Long values (usually
// message) are cut here; the writer surfaces one help hint about it.
const MaxValueLen = 500

// Projector resolves field paths against Datadog logs.
type Projector struct {
	Fields []string

	// Truncated reports whether any value was cut to MaxValueLen
	// since the projector was created.
	Truncated bool
}

// New returns a Projector for the given fields, or DefaultFields when
// none are given.
func New(fields []string) *Projector {
	if len(fields) == 0 {
		fields = DefaultFields
	}
	return &Projector{Fields: fields}
}

// Row projects a log to one string value per field, in field order.
// Missing fields resolve to "".
func (p *Projector) Row(log datadogV2.Log) []string {
	row := make([]string, len(p.Fields))
	for i, f := range p.Fields {
		v := p.resolve(log, f)
		if len(v) > MaxValueLen {
			cut := MaxValueLen
			for cut > 0 && !utf8.RuneStart(v[cut]) {
				cut--
			}
			v = v[:cut] + fmt.Sprintf("… (truncated, %d chars total)", len(v))
			p.Truncated = true
		}
		row[i] = v
	}
	return row
}

// Map projects a log to a field→value map (for json/ndjson output).
func (p *Projector) Map(log datadogV2.Log) map[string]string {
	m := make(map[string]string, len(p.Fields))
	row := p.Row(log)
	for i, f := range p.Fields {
		m[f] = row[i]
	}
	return m
}

// resolve maps a field name to its value. Reserved names hit the typed
// LogAttributes fields; anything else is a dot-separated path into the
// log's custom attributes.
func (p *Projector) resolve(log datadogV2.Log, field string) string {
	if field == "id" {
		return log.GetId()
	}
	attrs, ok := log.GetAttributesOk()
	if !ok {
		return ""
	}
	switch field {
	case "timestamp":
		if t, ok := attrs.GetTimestampOk(); ok {
			return t.UTC().Format(time.RFC3339)
		}
		return ""
	case "status":
		return attrs.GetStatus()
	case "service":
		return attrs.GetService()
	case "host":
		return attrs.GetHost()
	case "message":
		return attrs.GetMessage()
	case "tags":
		return strings.Join(attrs.GetTags(), " ")
	}
	// Custom attribute path, e.g. "http.status_code" or
	// "attributes.http.status_code" (explicit prefix allowed).
	path := strings.TrimPrefix(field, "attributes.")
	if v, ok := lookupPath(attrs.GetAttributes(), path); ok {
		return formatValue(v)
	}
	if v, ok := lookupPath(attrs.AdditionalProperties, path); ok {
		return formatValue(v)
	}
	return ""
}

// lookupPath walks a dot-separated path through nested maps.
func lookupPath(m map[string]any, path string) (any, bool) {
	if m == nil {
		return nil, false
	}
	// Prefer an exact key match (Datadog attributes may contain
	// literal dots in key names).
	if v, ok := m[path]; ok {
		return v, true
	}
	head, rest, found := strings.Cut(path, ".")
	if !found {
		return nil, false
	}
	child, ok := m[head].(map[string]any)
	if !ok {
		return nil, false
	}
	return lookupPath(child, rest)
}

func formatValue(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64:
		// JSON numbers decode as float64; print integers cleanly.
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%v", t)
	default:
		return fmt.Sprintf("%v", t)
	}
}
