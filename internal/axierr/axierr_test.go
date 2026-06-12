package axierr

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestRenderToon(t *testing.T) {
	var buf bytes.Buffer
	Render(&buf, "toon", Usage("missing_query", "--query is required",
		"dogfetch fetch --query 'service:web status:error'"))
	out := buf.String()
	if !strings.Contains(out, "error: \"--query is required (missing_query)\"") {
		t.Errorf("unexpected error line:\n%s", out)
	}
	if !strings.Contains(out, "help[1]:") || !strings.Contains(out, "dogfetch fetch --query") {
		t.Errorf("missing help block:\n%s", out)
	}
}

func TestRenderJSON(t *testing.T) {
	for _, format := range []string{"json", "ndjson"} {
		var buf bytes.Buffer
		Render(&buf, format, Runtime("auth_failed", "authentication failed", AuthHelp()...))
		var obj map[string]map[string]any
		if err := json.Unmarshal(buf.Bytes(), &obj); err != nil {
			t.Fatalf("%s: invalid JSON: %v", format, err)
		}
		e := obj["error"]
		if e["code"] != "auth_failed" || e["message"] != "authentication failed" {
			t.Errorf("%s: unexpected error object: %v", format, e)
		}
		if help, ok := e["help"].([]any); !ok || len(help) != 4 {
			t.Errorf("%s: unexpected help: %v", format, e["help"])
		}
	}
}

func TestErrorsAs(t *testing.T) {
	var target *Error
	err := error(Usage("x", "boom"))
	if !errors.As(err, &target) || target.Exit != ExitUsage {
		t.Error("errors.As should unwrap *Error")
	}
	if Runtime("y", "bang").Exit != ExitError {
		t.Error("Runtime exit code")
	}
}
