package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/jtzemp/dogfetch/internal/axierr"
	"github.com/jtzemp/dogfetch/internal/cli"
	"github.com/jtzemp/dogfetch/internal/config"
	"github.com/jtzemp/dogfetch/internal/toon"
)

// timeHelp is the shared remediation line for an unparseable --from/--to.
const timeHelp = "Use RFC3339 (2026-06-11T00:00:00Z), Unix seconds, or relative like 15m/2h/3d"

// commonEnvDefaults maps the flags every query subcommand shares to
// the env vars that can default them.
var commonEnvDefaults = map[string]string{
	"format": "DOGFETCH_FORMAT",
	"index":  "DOGFETCH_INDEX",
}

// parseFlags parses args. It returns (exit code, false) when the
// caller should return immediately: --help prints usage and exits 0, a
// bad flag renders a structured error and exits 2. flag writes its own
// diagnostic to stderr; per AXI the machine-readable block still has to
// reach stdout, or an agent sees only a bare exit code.
func parseFlags(fs *flag.FlagSet, args []string, format string) (int, bool) {
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK, false
		}
		outputFormat := format // in case there's a general parse error, but we have a valid format flag
		if f := fs.Lookup("format"); f != nil && f.Value.String() != f.DefValue {
			outputFormat = f.Value.String()
		}
		return fail(outputFormat, axierr.Usage(axierr.UsageCodeBadFlag, err.Error(),
			fmt.Sprintf("Run 'dogfetch %s --help' for the accepted flags", fs.Name()))), false
	}
	return exitOK, true
}

// applyEnvDefaults fills unset flags from DOGFETCH_* env vars,
// rendering a usage error in format when a value does not parse.
func applyEnvDefaults(fs *flag.FlagSet, mapping map[string]string, format string) (int, bool) {
	if err := cli.ApplyEnvDefaults(fs, mapping); err != nil {
		return fail(format, axierr.Usage(axierr.UsageCodeBadEnv, err.Error(),
			"Check DOGFETCH_* environment variables for invalid values")), false
	}
	return exitOK, true
}

// requireTOONOrJSON rejects the formats the aggregate commands cannot
// emit (ndjson is meaningless for a single aggregate document).
func requireTOONOrJSON(command, format string) *axierr.Error {
	if format == "toon" || format == "json" {
		return nil
	}
	return axierr.Usage(axierr.UsageCodeUsage,
		fmt.Sprintf("%s format must be 'toon' or 'json', got '%s'", command, format),
		fmt.Sprintf("dogfetch %s --query 'service:web' --from 2h --format toon", command))
}

// resolveQueryConfig builds the part of the config every subcommand
// shares: credentials (warnings go to errOut) and the --from/--to
// range. Callers fill in their own format/paging fields and run the
// validation their flags require.
func resolveQueryConfig(query, index, from, to string, errOut io.Writer) (*config.Config, *axierr.Error) {
	creds := config.ResolveCredentials()
	for _, warning := range creds.Warnings {
		fmt.Fprintf(errOut, "Warning: %s\n", warning)
	}

	cfg := &config.Config{
		Query:  query,
		Index:  index,
		APIKey: creds.APIKey,
		AppKey: creds.AppKey,
		Site:   creds.Site,
	}

	if from != "" {
		parsed, err := config.ParseTime(from)
		if err != nil {
			return nil, axierr.Usage(axierr.UsageCodeBadTime, fmt.Sprintf("invalid --from: %v", err), timeHelp)
		}
		cfg.From = parsed
	} else {
		cfg.From = config.DefaultFrom()
	}

	if to != "" {
		parsed, err := config.ParseTime(to)
		if err != nil {
			return nil, axierr.Usage(axierr.UsageCodeBadTime, fmt.Sprintf("invalid --to: %v", err), timeHelp)
		}
		cfg.To = parsed
	}

	return cfg, nil
}

// validateCredentials runs the site and auth checks shared by every
// command that talks to the API.
func validateCredentials(cfg *config.Config) *axierr.Error {
	if err := config.ValidateSite(cfg.Site); err != nil {
		return axierr.Usage(axierr.UsageCodeBadSite, err.Error(),
			"Set DD_SITE to a domain like datadoghq.com or datadoghq.eu")
	}
	if err := cfg.ValidateAuth(); err != nil {
		return axierr.Runtime("auth_missing", err.Error(), axierr.AuthHelp()...)
	}
	return nil
}

// asAXIError unwraps err to its structured form, falling back to a
// runtime error under code when the error carries no AXI shape.
func asAXIError(err error, code string) *axierr.Error {
	var ae *axierr.Error
	if errors.As(err, &ae) {
		return ae
	}
	return axierr.Runtime(code, err.Error())
}

// interruptContext returns a context cancelled on Ctrl+C, so an
// in-flight fetch can shut down and report a resume cursor.
func interruptContext(errOut io.Writer) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())

	sigChan := make(chan os.Signal, 1)
	// os.Interrupt works on both Unix and Windows (Ctrl+C)
	signal.Notify(sigChan, os.Interrupt)

	go func() {
		<-sigChan
		fmt.Fprintf(errOut, "\nReceived interrupt signal, shutting down gracefully...\n")
		cancel()
	}()

	return ctx, cancel
}

// encStatus maps an encoder's first write error to an exit code.
func encStatus(enc *toon.Encoder) int {
	if enc.Err() != nil {
		return exitError
	}
	return exitOK
}
