package toon

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

func checkGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("update golden %s: %v", path, err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run `go test ./internal/toon -update` to create)", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("output mismatch for %s\ngot:\n%s\nwant:\n%s", name, got, want)
	}
}

func TestLogsOutput(t *testing.T) {
	var buf bytes.Buffer
	e := NewEncoder(&buf)
	e.Scalar("count", 3)
	e.Table("logs", []string{"timestamp", "status", "service", "message"}, [][]string{
		{"2026-06-11T10:00:00Z", "error", "web", "connection refused"},
		{"2026-06-11T10:00:01Z", "warn", "api", `timeout after 5s, retrying`},
		{"2026-06-11T10:00:02Z", "info", "web", `user "bob" logged in`},
	})
	e.List("help", []string{
		"Widen output with --fields timestamp,status,service,message,host",
		"Resume with --cursor '<cursor>'",
	})
	if err := e.Err(); err != nil {
		t.Fatal(err)
	}
	checkGolden(t, "logs.toon", buf.Bytes())
}

func TestEmptyTable(t *testing.T) {
	var buf bytes.Buffer
	e := NewEncoder(&buf)
	e.Scalar("logs", `0 matched query "service:web" in range 2026-06-10T10:00:00Z to now`)
	if err := e.Err(); err != nil {
		t.Fatal(err)
	}
	checkGolden(t, "empty.toon", buf.Bytes())
}

func TestQuote(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"plain", "plain"},
		{"two words", "two words"},
		{"", `""`},
		{" padded ", `" padded "`},
		{"true", `"true"`},
		{"false", `"false"`},
		{"null", `"null"`},
		{"42", `"42"`},
		{"-1.5e3", `"-1.5e3"`},
		{"1.2.3", "1.2.3"},
		{"a,b", `"a,b"`},
		{"key: value", `"key: value"`},
		{"trailing:", `"trailing:"`},
		{"2026-06-11T10:00:00Z", "2026-06-11T10:00:00Z"},
		{"https://api.datadoghq.com", "https://api.datadoghq.com"},
		{"-flag", `"-flag"`},
		{"#hash", `"#hash"`},
		{"[bracket", `"[bracket"`},
		{"{brace", `"{brace"`},
		{`say "hi"`, `"say \"hi\""`},
		{"line1\nline2", `"line1\nline2"`},
		{"tab\there", `"tab\there"`},
		{`back\slash`, `"back\\slash"`},
	}
	for _, tt := range tests {
		if got := Quote(tt.in); got != tt.want {
			t.Errorf("Quote(%q) = %s, want %s", tt.in, got, tt.want)
		}
	}
}

func TestListEmptyOmitted(t *testing.T) {
	var buf bytes.Buffer
	e := NewEncoder(&buf)
	e.List("help", nil)
	if buf.Len() != 0 {
		t.Errorf("empty list should emit nothing, got %q", buf.String())
	}
}
