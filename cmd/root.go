package cmd

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/jtzemp/dogfetch/internal/config"
	"github.com/jtzemp/dogfetch/internal/fetcher"
)

// Execute runs the CLI
func Execute() {
	// Define flags
	query := flag.String("query", "", "The filter query (search term)")
	index := flag.String("index", "main", "Which index to read from")
	from := flag.String("from", "", "Start date/time (default: 24 hours ago)")
	to := flag.String("to", "", "End date/time (default: now)")
	pageSize := flag.Int("pageSize", 1000, "Results per page (max 5000)")
	output := flag.String("output", "results.json", "Output file path")
	format := flag.String("format", "json", "Output format: json or ndjson")
	cursor := flag.String("cursor", "", "Page cursor for resuming")
	appendFlag := flag.Bool("append", false, "Append to output file (ndjson only)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "dogfetch - Fetch logs from Datadog\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  dogfetch --query 'service:web status:error'\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nEnvironment Variables:\n")
		fmt.Fprintf(os.Stderr, "  DD_API_KEY   Datadog API key (required)\n")
		fmt.Fprintf(os.Stderr, "  DD_APP_KEY   Datadog Application key (required)\n")
		fmt.Fprintf(os.Stderr, "  DD_SITE      Datadog site (optional, default: datadoghq.com)\n")
	}

	flag.Parse()

	// Build config
	cfg := &config.Config{
		Query:      *query,
		Index:      *index,
		PageSize:   int32(*pageSize),
		OutputPath: *output,
		Format:     *format,
		Cursor:     *cursor,
		Append:     *appendFlag,
		APIKey:     os.Getenv("DD_API_KEY"),
		AppKey:     os.Getenv("DD_APP_KEY"),
		Site:       os.Getenv("DD_SITE"),
	}

	// Parse time range
	if *from != "" {
		parsedFrom, err := config.ParseTime(*from)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing --from: %v\n", err)
			os.Exit(1)
		}
		cfg.From = parsedFrom
	} else {
		cfg.From = config.DefaultFrom()
	}

	if *to != "" {
		parsedTo, err := config.ParseTime(*to)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing --to: %v\n", err)
			os.Exit(1)
		}
		cfg.To = parsedTo
	}

	// Validate config
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		flag.Usage()
		os.Exit(1)
	}

	// Create fetcher
	f, err := fetcher.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create fetcher: %v\n", err)
		os.Exit(1)
	}

	// Setup signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\nReceived interrupt signal, shutting down gracefully...")
		cancel()
	}()

	// Execute fetch
	if err := f.Fetch(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Fetch failed: %v\n", err)
		os.Exit(1)
	}
}
