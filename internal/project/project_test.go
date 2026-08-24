package project

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

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
	// 600 multibyte runes (1200 bytes): over the rune limit, so it is
	// cut, and the reported total must be runes rather than bytes.
	long := strings.Repeat("é", 600)
	l := testLog()
	l.Attributes.Message = &long

	p := New(nil)
	row := p.Row(l)
	msg := row[3]
	if !p.Truncated {
		t.Error("expected Truncated flag")
	}
	if !strings.Contains(msg, "truncated, 600 chars total") {
		t.Errorf("total must be counted in runes, not bytes: %q", msg)
	}
	prefix, _, _ := strings.Cut(msg, "…")
	if !strings.HasPrefix(long, prefix) {
		t.Errorf("cut is not on a rune boundary: %q", prefix)
	}
	if n := utf8.RuneCountInString(prefix); n != MaxValueLen {
		t.Errorf("kept %d runes, want %d", n, MaxValueLen)
	}
}

// TestTruncationCountsRunesNotBytes pins the bug Copilot caught: a
// value under the rune limit must survive intact even when its UTF-8
// encoding pushes it past MaxValueLen bytes.
func TestTruncationCountsRunesNotBytes(t *testing.T) {
	long := strings.Repeat("é", 400) // 400 runes, 800 bytes
	l := testLog()
	l.Attributes.Message = &long

	p := New(nil)
	got := p.Row(l)[3]
	if p.Truncated {
		t.Error("400 runes is under the 500-rune limit; must not truncate")
	}
	if got != long {
		t.Errorf("value was altered: got %d bytes, want %d", len(got), len(long))
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
