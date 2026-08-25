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
		Render(&buf, format, Runtime("auth_failed", "authentication failed", AuthHelp("")...))
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

func TestAuthHelpSiteAware(t *testing.T) {
	tests := []struct {
		name, site, wantHost string
	}{
		{"empty site defaults to US1", "", "app.datadoghq.com"},
		{"us3", "us3.datadoghq.com", "app.us3.datadoghq.com"},
		{"eu", "datadoghq.eu", "app.datadoghq.eu"},
		{"gov", "ddog-gov.com", "app.ddog-gov.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, help := range [][]string{AuthHelp(tt.site), AuthSetupHelp(tt.site)} {
				want := "https://" + tt.wantHost + "/organization-settings/api-keys"
				found := false
				for _, line := range help {
					if strings.Contains(line, want) {
						found = true
					}
					if strings.Contains(line, "app.datadoghq.com") && tt.wantHost != "app.datadoghq.com" {
						t.Errorf("help leaked default-site host for site %q: %q", tt.site, line)
					}
				}
				if !found {
					t.Errorf("site %q: help missing %q, got %v", tt.site, want, help)
				}
			}
		})
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
