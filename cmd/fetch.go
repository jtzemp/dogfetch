package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/jtzemp/dogfetch/internal/cli"
	"github.com/jtzemp/dogfetch/internal/config"
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
	fields := fs.String("fields", "", "Comma-separated fields to include in output")
	output := fs.String("output", "", "Output file path (default: stdout)")
	format := fs.String("format", "ndjson", "Output format: json or ndjson")
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

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}

	// Handle version flag
	if *versionFlag {
		fmt.Println(version.Info())
		return exitOK
	}

	// Flag > env > default precedence
	if err := cli.ApplyEnvDefaults(fs, fetchEnvDefaults); err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		return exitUsage
	}

	// Setup error output
	errOut := os.Stderr
	if *errorsOut != "" {
		f, err := os.OpenFile(*errorsOut, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to open error output file: %v\n", err)
			return exitError
		}
		defer f.Close()
		errOut = f
	}

	// Resolve credentials: process env first, ~/.config/dogfetch/env fallback
	creds := config.ResolveCredentials()
	for _, warning := range creds.Warnings {
		fmt.Fprintf(errOut, "Warning: %s\n", warning)
	}

	// Build config
	cfg := &config.Config{
		Query:      *query,
		Index:      *index,
		PageSize:   int32(*pageSize),
		Limit:      *limit,
		OutputPath: *output,
		Format:     *format,
		Cursor:     *cursor,
		Append:     *appendFlag,
		Fields:     splitFields(*fields),
		APIKey:     creds.APIKey,
		AppKey:     creds.AppKey,
		Site:       creds.Site,
	}

	// Parse time range
	if *from != "" {
		parsedFrom, err := config.ParseTime(*from)
		if err != nil {
			fmt.Fprintf(errOut, "Error parsing --from: %v\n", err)
			return exitUsage
		}
		cfg.From = parsedFrom
	} else {
		cfg.From = config.DefaultFrom()
	}

	if *to != "" {
		parsedTo, err := config.ParseTime(*to)
		if err != nil {
			fmt.Fprintf(errOut, "Error parsing --to: %v\n", err)
			return exitUsage
		}
		cfg.To = parsedTo
	}

	// Validate config
	if err := cfg.ValidateUsage(); err != nil {
		fmt.Fprintf(errOut, "Configuration error: %v\n", err)
		fs.Usage()
		return exitUsage
	}

	if err := cfg.ValidateAuth(); err != nil {
		fmt.Fprintf(errOut, "Authentication error: %v\n", err)
		fmt.Fprintf(errOut, "Set DD_API_KEY and DD_APP_KEY in the environment or in ~/.config/dogfetch/env\n")
		return exitError
	}

	// Create fetcher
	f, err := fetcher.New(cfg, errOut)
	if err != nil {
		fmt.Fprintf(errOut, "Failed to create fetcher: %v\n", err)
		return exitError
	}

	// Setup signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	// os.Interrupt works on both Unix and Windows (Ctrl+C)
	signal.Notify(sigChan, os.Interrupt)

	go func() {
		<-sigChan
		fmt.Fprintf(errOut, "\nReceived interrupt signal, shutting down gracefully...\n")
		cancel()
	}()

	// Execute fetch
	if _, err := f.Fetch(ctx); err != nil {
		fmt.Fprintf(errOut, "Fetch failed: %v\n", err)
		return exitError
	}

	return exitOK
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
