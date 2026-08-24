// Package axierr defines structured, agent-readable errors. Per AXI,
// errors render on stdout in the active output format with actionable
// help, while raw diagnostics stay on stderr.
package axierr

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/jtzemp/dogfetch/internal/toon"
)

// Exit codes follow the AXI convention.
const (
	ExitOK    = 0
	ExitError = 1 // runtime error
	ExitUsage = 2 // usage error
)

// Error is a structured CLI error.
type Error struct {
	Code    string   // stable machine-readable code, e.g. "auth_failed"
	Message string   // one-line human/agent description
	Help    []string // actionable next steps (complete commands/URLs)
	Exit    int      // process exit code
}

func (e *Error) Error() string {
	return e.Message
}

// Usage builds a usage error (exit 2).
func Usage(code, message string, help ...string) *Error {
	return &Error{Code: code, Message: message, Help: help, Exit: ExitUsage}
}

// Runtime builds a runtime error (exit 1).
func Runtime(code, message string, help ...string) *Error {
	return &Error{Code: code, Message: message, Help: help, Exit: ExitError}
}

// authSteps are the credential setup steps both help blocks share.
func authSteps() []string {
	return []string{
		"Create an API key at https://app.datadoghq.com/organization-settings/api-keys",
		"Create an Application key at https://app.datadoghq.com/organization-settings/application-keys",
		"Export DD_API_KEY and DD_APP_KEY, or store them in ~/.config/dogfetch/env (KEY=VALUE lines, chmod 600)",
	}
}

// AuthHelp is the remediation block for missing/rejected credentials,
// for commands that are not themselves `dogfetch auth`.
func AuthHelp() []string {
	return append(authSteps(), "Run `dogfetch auth` to check credential status")
}

// AuthSetupHelp is the block `dogfetch auth` itself shows: pointing a
// reader back at the command they already ran helps nobody, so the
// last step spells out the env-file format instead.
func AuthSetupHelp() []string {
	return append(authSteps(),
		"Example ~/.config/dogfetch/env:  DD_API_KEY=<key>  DD_APP_KEY=<key>  DD_SITE=datadoghq.com  (one per line)")
}

// Render writes the error to w in the given output format. Formats
// "json" and "ndjson" emit a single JSON object; anything else
// (including "toon") emits a TOON error block.
func Render(w io.Writer, format string, e *Error) {
	switch format {
	case "json", "ndjson":
		obj := map[string]any{
			"error": map[string]any{
				"code":    e.Code,
				"message": e.Message,
				"help":    e.Help,
			},
		}
		enc := json.NewEncoder(w)
		if format == "json" {
			enc.SetIndent("", "  ")
		}
		_ = enc.Encode(obj)
	default:
		enc := toon.NewEncoder(w)
		enc.Scalar("error", fmt.Sprintf("%s (%s)", e.Message, e.Code))
		enc.List("help", e.Help)
		_ = enc.Err() // flushes; nothing useful to do if stdout is gone
	}
}
