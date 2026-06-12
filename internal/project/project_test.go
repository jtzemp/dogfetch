package project

import (
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
)

func testLog() datadogV2.Log {
	id := "AQAAAY"
	host := "web-1"
	msg := "connection refused"
	svc := "web"
	status := "error"
	ts := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	return datadogV2.Log{
		Id: &id,
		Attributes: &datadogV2.LogAttributes{
			Host:      &host,
			Message:   &msg,
			Service:   &svc,
			Status:    &status,
			Tags:      []string{"env:prod", "team:core"},
			Timestamp: &ts,
			Attributes: map[string]any{
				"http": map[string]any{
					"status_code": float64(502),
					"url":         "https://example.com/x",
				},
				"dotted.key": "literal",
				"latency_ms": 12.5,
			},
		},
	}
}

func TestDefaultProjection(t *testing.T) {
	p := New(nil)
	row := p.Row(testLog())
	want := []string{"2026-06-11T10:00:00Z", "error", "web", "connection refused"}
	for i, w := range want {
		if row[i] != w {
			t.Errorf("field %s = %q, want %q", p.Fields[i], row[i], w)
		}
	}
}

func TestFieldResolution(t *testing.T) {
	tests := []struct {
		field, want string
	}{
		{"id", "AQAAAY"},
		{"host", "web-1"},
		{"tags", "env:prod team:core"},
		{"http.status_code", "502"},
		{"http.url", "https://example.com/x"},
		{"attributes.http.status_code", "502"},
		{"dotted.key", "literal"},
		{"latency_ms", "12.5"},
		{"missing", ""},
		{"http.nope", ""},
	}
	for _, tt := range tests {
		p := New([]string{tt.field})
		if got := p.Row(testLog())[0]; got != tt.want {
			t.Errorf("resolve(%q) = %q, want %q", tt.field, got, tt.want)
		}
	}
}

func TestTruncation(t *testing.T) {
	long := strings.Repeat("é", 400) // 800 bytes, multibyte
	l := testLog()
	l.Attributes.Message = &long

	p := New(nil)
	row := p.Row(l)
	msg := row[3]
	if !p.Truncated {
		t.Error("expected Truncated flag")
	}
	if !strings.Contains(msg, "truncated, 800 chars total") {
		t.Errorf("missing truncation marker: %q", msg)
	}
	prefix, _, _ := strings.Cut(msg, "…")
	if !strings.HasPrefix(long, prefix) || len(prefix) > MaxValueLen {
		t.Errorf("bad rune-boundary cut, prefix len %d", len(prefix))
	}
}

func TestNoTruncationFlagWhenShort(t *testing.T) {
	p := New(nil)
	p.Row(testLog())
	if p.Truncated {
		t.Error("Truncated should stay false for short values")
	}
}

func TestMap(t *testing.T) {
	p := New([]string{"status", "service"})
	m := p.Map(testLog())
	if m["status"] != "error" || m["service"] != "web" {
		t.Errorf("unexpected map: %v", m)
	}
}

func TestNilAttributes(t *testing.T) {
	p := New(nil)
	row := p.Row(datadogV2.Log{})
	for i, v := range row {
		if v != "" {
			t.Errorf("field %s = %q, want empty", p.Fields[i], v)
		}
	}
}
