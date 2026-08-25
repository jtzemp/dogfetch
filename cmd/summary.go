package cmd

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/jtzemp/dogfetch/internal/axierr"
	"github.com/jtzemp/dogfetch/internal/config"
	"github.com/jtzemp/dogfetch/internal/fetcher"
	"github.com/jtzemp/dogfetch/internal/toon"
)

func runSummary(args []string) int {
	fs := flag.NewFlagSet("summary", flag.ContinueOnError)

	query := fs.String("query", "", "The filter query (default: all logs)")
	index := fs.String("index", "main", "Which index to read from")
	from := fs.String("from", "", "Start date/time: RFC3339, Unix seconds, or relative like 15m/2h/3d (default: 24 hours ago)")
	to := fs.String("to", "", "End date/time (default: now)")
	format := fs.String("format", "toon", "Output format: toon or json")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "dogfetch summary - Pre-computed aggregates: total, by status, by service, timeline\n\n")
		fmt.Fprintf(os.Stderr, "Uses the Datadog Aggregate API: no pagination, fast, no raw logs.\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  dogfetch summary --query 'service:web' --from 2h\n")
		fmt.Fprintf(os.Stderr, "  dogfetch summary --query 'status:error' --from 1d\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fs.PrintDefaults()
	}

	if code, ok := parseFlags(fs, args, *format); !ok {
		return code
	}

	if code, ok := applyEnvDefaults(fs, commonEnvDefaults, *format); !ok {
		return code
	}

	if aerr := requireTOONOrJSON("summary", *format); aerr != nil {
		return fail("toon", aerr)
	}

	cfg, aerr := resolveQueryConfig(*query, *index, *from, *to, os.Stderr)
	if aerr != nil {
		return fail(*format, aerr)
	}

	if err := cfg.ValidateRange(); err != nil {
		return fail(*format, axierr.Usage(axierr.UsageCodeUsage, err.Error()))
	}

	if aerr := validateCredentials(cfg); aerr != nil {
		return fail(*format, aerr)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	summary, err := fetcher.NewAggregator(cfg, os.Stderr).Summarize(ctx)
	if err != nil {
		return fail(*format, asAXIError(err, "aggregate_failed"))
	}

	if *format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(summary); err != nil {
			return exitError
		}
		return exitOK
	}

	return renderSummaryToon(summary, cfg)
}

// renderSummaryToon writes the TOON summary view.
func renderSummaryToon(s *fetcher.Summary, cfg *config.Config) int {
	enc := toon.NewEncoder(os.Stdout)

	queryDisplay := cfg.Query
	if queryDisplay == "" {
		queryDisplay = "*"
	}

	if s.IsEmpty() {
		toon.EmptyState(enc, "total", "", queryDisplay, cfg.From, cfg.To)
		return encStatus(enc)
	}

	enc.Scalar("total", s.Total)
	enc.Table("by_status", []string{"status", "count"}, facetRows(s.ByStatus))
	enc.Table("by_service", []string{"service", "count"}, facetRows(s.ByService))
	enc.Table("timeline", []string{"time", "count"}, timelineRows(s.Timeline))

	help := []string{}
	if s.StatusCardinality > int64(len(s.ByStatus)) {
		help = append(help, fmt.Sprintf("by_status shows top %d of %d statuses", len(s.ByStatus), s.StatusCardinality))
	}
	if s.ServiceCardinality > int64(len(s.ByService)) {
		help = append(help, fmt.Sprintf("by_service shows top %d of %d services; narrow with --query '%s service:<name>'",
			len(s.ByService), s.ServiceCardinality, queryDisplay))
	}
	help = append(help,
		fmt.Sprintf("Group repetitive logs: dogfetch patterns --query '%s status:<status>'", queryDisplay),
		fmt.Sprintf("See raw logs: dogfetch fetch --query '%s status:<status>' --limit 50", queryDisplay))
	enc.List("help", help)

	return encStatus(enc)
}

func facetRows(counts []fetcher.FacetCount) [][]any {
	rows := make([][]any, len(counts))
	for i, c := range counts {
		rows[i] = []any{c.Value, c.Count}
	}
	return rows
}

func timelineRows(points []fetcher.TimePoint) [][]any {
	rows := make([][]any, len(points))
	for i, p := range points {
		rows[i] = []any{p.Time, p.Count}
	}
	return rows
}
