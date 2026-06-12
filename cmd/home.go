package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/jtzemp/dogfetch/internal/config"
	"github.com/jtzemp/dogfetch/internal/toon"
)

// runHome is the AXI content-first no-args view: identify the tool,
// show live auth state, and suggest next commands.
func runHome() int {
	enc := toon.NewEncoder(os.Stdout)
	enc.Scalar("bin", binPath())
	enc.Scalar("description", "Fetch Datadog logs as agent-friendly TOON (json/ndjson for full export)")
	enc.Scalar("auth", authStatus(config.ResolveCredentials()))
	enc.List("help", []string{
		"dogfetch summary --query 'service:web' --from 2h   (counts by status/service + timeline, no raw logs)",
		"dogfetch fetch --query 'service:web status:error' --from 2h --limit 100",
		"dogfetch fetch --query '<query>' --output logs.ndjson   (full export)",
		"dogfetch fetch --fields timestamp,status,service,message,host --query '<query>'",
		"dogfetch auth      (credential status and setup help)",
		"dogfetch version",
	})
	if enc.Err() != nil {
		return exitError
	}
	return exitOK
}

// binPath returns the current executable path with $HOME collapsed to ~.
func binPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "dogfetch"
	}
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(exe, home) {
		return "~" + strings.TrimPrefix(exe, home)
	}
	return exe
}

// authStatus summarizes credential state in one line.
func authStatus(creds config.Credentials) string {
	missing := []string{}
	if creds.APIKey == "" {
		missing = append(missing, "DD_API_KEY")
	}
	if creds.AppKey == "" {
		missing = append(missing, "DD_APP_KEY")
	}
	if len(missing) > 0 {
		return "missing " + strings.Join(missing, " and ")
	}
	site := creds.Site
	if site == "" {
		site = "datadoghq.com"
	}
	return fmt.Sprintf("ok (site %s, source %s)", site, creds.Source)
}
