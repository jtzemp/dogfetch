package cmd

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/jtzemp/dogfetch/internal/axierr"
	"github.com/jtzemp/dogfetch/internal/fetcher"
	"github.com/jtzemp/dogfetch/internal/version"
)

// fetchEnvDefaults maps fetch flags to the env vars that can default them.
var fetchEnvDefaults = map[string]string{
	"format":   "DOGFETCH_FORMAT",
	"fields":   "DOGFETCH_FIELDS",
	"limit":    "DOGFETCH_LIMIT",
	"pageSize": "DOGFETCH_PAGESIZE",
	"index":    "DOGFETCH_INDEX",
}

func runFetch(args []string) int {
	fs := flag.NewFlagSet("fetch", flag.ContinueOnError)

	versionFlag := fs.Bool("version", false, "Print version information")
	query := fs.String("query", "", "The filter query (search term)")
	index := fs.String("index", "main", "Which index to read from")
	from := fs.String("from", "", "Start date/time: RFC3339, Unix seconds, or relative like 15m/2h/3d (default: 24 hours ago)")
	to := fs.String("to", "", "End date/time (default: now)")
	pageSize := fs.Int("pageSize", 1000, "Results per page (max 5000)")
	limit := fs.Int("limit", 0, "Stop after this many logs (0 = unlimited)")
	fields := fs.String("fields", "", "Comma-separated fields to include in output (default: timestamp,status,service,message for toon)")
	output := fs.String("output", "", "Output file path (default: stdout)")
	format := fs.String("format", "", "Output format: toon, json, or ndjson (default: toon on stdout, ndjson with --output)")
	cursor := fs.String("cursor", "", "Page cursor for resuming")
	appendFlag := fs.Bool("append", false, "Append to output file (ndjson only)")
	errorsOut := fs.String("errors-out", "", "Write errors to file (default: stderr)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "dogfetch - Fetch logs from Datadog\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  dogfetch fetch --query 'service:web status:error'\n")
		fmt.Fprintf(os.Stderr, "  dogfetch fetch --query 'service:web' --from 2h --limit 100\n")
		fmt.Fprintf(os.Stderr, "  dogfetch fetch --query 'service:web' --output logs.ndjson\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nEnvironment Variables:\n")
		fmt.Fprintf(os.Stderr, "  DD_API_KEY         Datadog API key (required; also read from ~/.config/dogfetch/env)\n")
		fmt.Fprintf(os.Stderr, "  DD_APP_KEY         Datadog Application key (required; also read from ~/.config/dogfetch/env)\n")
		fmt.Fprintf(os.Stderr, "  DD_SITE            Datadog site (optional, default: datadoghq.com)\n")
		fmt.Fprintf(os.Stderr, "  DOGFETCH_FORMAT    Default for --format\n")
		fmt.Fprintf(os.Stderr, "  DOGFETCH_FIELDS    Default for --fields\n")
		fmt.Fprintf(os.Stderr, "  DOGFETCH_LIMIT     Default for --limit\n")
		fmt.Fprintf(os.Stderr, "  DOGFETCH_PAGESIZE  Default for --pageSize\n")
		fmt.Fprintf(os.Stderr, "  DOGFETCH_INDEX     Default for --index\n")
	}

	if code, ok := parseFlags(fs, args); !ok {
		return code
	}

	// Handle version flag
	if *versionFlag {
		fmt.Println(version.Info())
		return exitOK
	}

	// Flag > env > default precedence
	if code, ok := applyEnvDefaults(fs, fetchEnvDefaults, *format); !ok {
		return code
	}

	// Default format (D6): agent-friendly toon on stdout, lossless
	// ndjson when exporting to a file.
	if *format == "" {
		if *output != "" {
			*format = "ndjson"
		} else {
			*format = "toon"
		}
	}

	// Setup error output
	errOut := os.Stderr
	if *errorsOut != "" {
		f, err := os.OpenFile(*errorsOut, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return fail(*format, axierr.Runtime("bad_errors_out",
				fmt.Sprintf("failed to open --errors-out file: %v", err)))
		}
		defer f.Close()
		errOut = f
	}

	// Resolve credentials and the time range (shared with summary/patterns)
	cfg, aerr := resolveQueryConfig(*query, *index, *from, *to, errOut)
	if aerr != nil {
		return fail(*format, aerr)
	}
	cfg.PageSize = int32(*pageSize)
	cfg.Limit = *limit
	cfg.OutputPath = *output
	cfg.Format = *format
	cfg.Cursor = *cursor
	cfg.Append = *appendFlag
	cfg.Fields = splitFields(*fields)

	// Validate config
	if err := cfg.ValidateUsage(); err != nil {
		return fail(*format, axierr.Usage("usage", err.Error(),
			"dogfetch fetch --query 'service:web status:error' --from 2h --limit 100"))
	}

	if aerr := validateCredentials(cfg); aerr != nil {
		return fail(*format, aerr)
	}

	// Create fetcher
	f, err := fetcher.New(cfg, errOut)
	if err != nil {
		return fail(*format, axierr.Runtime("init_failed",
			fmt.Sprintf("failed to create fetcher: %v", err)))
	}

	// Setup signal handling for graceful shutdown
	ctx, cancel := interruptContext(errOut)
	defer cancel()

	// Execute fetch
	if _, err := f.Fetch(ctx); err != nil {
		fmt.Fprintf(errOut, "Fetch failed: %v\n", err)
		return fail(*format, asAXIError(err, "fetch_failed"))
	}

	return exitOK
}

// fail renders a structured error on stdout (per AXI) and returns its
// exit code.
func fail(format string, e *axierr.Error) int {
	axierr.Render(os.Stdout, format, e)
	return e.Exit
}

func splitFields(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	fields := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			fields = append(fields, p)
		}
	}
	return fields
}
