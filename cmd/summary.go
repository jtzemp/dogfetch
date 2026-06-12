package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/jtzemp/dogfetch/internal/axierr"
	"github.com/jtzemp/dogfetch/internal/cli"
	"github.com/jtzemp/dogfetch/internal/config"
	"github.com/jtzemp/dogfetch/internal/fetcher"
	"github.com/jtzemp/dogfetch/internal/toon"
)

// summaryEnvDefaults maps summary flags to env vars.
var summaryEnvDefaults = map[string]string{
	"format": "DOGFETCH_FORMAT",
	"index":  "DOGFETCH_INDEX",
}

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

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}

	if err := cli.ApplyEnvDefaults(fs, summaryEnvDefaults); err != nil {
		return fail(*format, axierr.Usage("bad_env", err.Error(),
			"Check DOGFETCH_* environment variables for invalid values"))
	}

	if *format != "toon" && *format != "json" {
		return fail("toon", axierr.Usage("usage",
			fmt.Sprintf("summary format must be 'toon' or 'json', got '%s'", *format),
			"dogfetch summary --query 'service:web' --from 2h --format toon"))
	}

	creds := config.ResolveCredentials()
	for _, warning := range creds.Warnings {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", warning)
	}

	cfg := &config.Config{
		Query:  *query,
		Index:  *index,
		APIKey: creds.APIKey,
		AppKey: creds.AppKey,
		Site:   creds.Site,
	}

	if *from != "" {
		parsed, err := config.ParseTime(*from)
		if err != nil {
			return fail(*format, axierr.Usage("bad_time", fmt.Sprintf("invalid --from: %v", err),
				"Use RFC3339 (2026-06-11T00:00:00Z), Unix seconds, or relative like 15m/2h/3d"))
		}
		cfg.From = parsed
	} else {
		cfg.From = config.DefaultFrom()
	}

	if *to != "" {
		parsed, err := config.ParseTime(*to)
		if err != nil {
			return fail(*format, axierr.Usage("bad_time", fmt.Sprintf("invalid --to: %v", err),
				"Use RFC3339 (2026-06-11T00:00:00Z), Unix seconds, or relative like 15m/2h/3d"))
		}
		cfg.To = parsed
	}

	if !cfg.To.IsZero() && cfg.From.After(cfg.To) {
		return fail(*format, axierr.Usage("usage",
			fmt.Sprintf("--from (%s) must be before --to (%s)", cfg.From, cfg.To)))
	}

	if err := cfg.ValidateAuth(); err != nil {
		return fail(*format, axierr.Runtime("auth_missing", err.Error(), axierr.AuthHelp()...))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	summary, err := fetcher.NewAggregator(cfg, os.Stderr).Summarize(ctx)
	if err != nil {
		var ae *axierr.Error
		if !errors.As(err, &ae) {
			ae = axierr.Runtime("aggregate_failed", err.Error())
		}
		return fail(*format, ae)
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

	if s.Total == 0 {
		enc.Scalar("total", fmt.Sprintf("0 matched query '%s' in range %s to %s",
			queryDisplay, formatRangeTime(cfg.From), formatRangeTime(cfg.To)))
		enc.List("help", []string{
			"Widen the time range with --from 24h or loosen the query",
		})
		if enc.Err() != nil {
			return exitError
		}
		return exitOK
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

	if enc.Err() != nil {
		return exitError
	}
	return exitOK
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

func formatRangeTime(t time.Time) string {
	if t.IsZero() {
		return "now"
	}
	return t.UTC().Format(time.RFC3339)
}
