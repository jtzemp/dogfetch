package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	"github.com/jtzemp/dogfetch/internal/axierr"
	"github.com/jtzemp/dogfetch/internal/cli"
	"github.com/jtzemp/dogfetch/internal/cluster"
	"github.com/jtzemp/dogfetch/internal/config"
	"github.com/jtzemp/dogfetch/internal/fetcher"
	"github.com/jtzemp/dogfetch/internal/toon"
	"github.com/jtzemp/dogfetch/internal/writer"
)

// patternsEnvDefaults maps patterns flags to env vars.
var patternsEnvDefaults = map[string]string{
	"format": "DOGFETCH_FORMAT",
	"index":  "DOGFETCH_INDEX",
}

func runPatterns(args []string) int {
	fs := flag.NewFlagSet("patterns", flag.ContinueOnError)

	query := fs.String("query", "", "The filter query (default: all logs)")
	index := fs.String("index", "main", "Which index to read from")
	from := fs.String("from", "", "Start date/time: RFC3339, Unix seconds, or relative like 15m/2h/3d (default: 24 hours ago)")
	to := fs.String("to", "", "End date/time (default: now)")
	limit := fs.Int("limit", 10000, "Scan at most this many logs (0 = unlimited)")
	top := fs.Int("top", 50, "Show only the most frequent N patterns (0 = all)")
	samples := fs.Bool("samples", false, "Include one sample message per pattern")
	format := fs.String("format", "toon", "Output format: toon or json")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "dogfetch patterns - Cluster log messages into templates (drain-style)\n\n")
		fmt.Fprintf(os.Stderr, "Volatile tokens (numbers, ids, IPs, quoted values) become <*>, so\n")
		fmt.Fprintf(os.Stderr, "thousands of repetitive logs collapse into a handful of patterns.\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  dogfetch patterns --query 'service:web status:error' --from 2h\n")
		fmt.Fprintf(os.Stderr, "  dogfetch patterns --query 'service:web' --samples --top 20\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}

	if err := cli.ApplyEnvDefaults(fs, patternsEnvDefaults); err != nil {
		return fail(*format, axierr.Usage("bad_env", err.Error(),
			"Check DOGFETCH_* environment variables for invalid values"))
	}

	if *format != "toon" && *format != "json" {
		return fail("toon", axierr.Usage("usage",
			fmt.Sprintf("patterns format must be 'toon' or 'json', got '%s'", *format),
			"dogfetch patterns --query 'service:web' --from 2h --format toon"))
	}
	if *limit < 0 || *top < 0 {
		return fail(*format, axierr.Usage("usage", "--limit and --top must be >= 0"))
	}

	creds := config.ResolveCredentials()
	for _, warning := range creds.Warnings {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", warning)
	}

	cfg := &config.Config{
		Query:    *query,
		Index:    *index,
		PageSize: 1000,
		Limit:    *limit,
		APIKey:   creds.APIKey,
		AppKey:   creds.AppKey,
		Site:     creds.Site,
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

	collector := &clusterCollector{clusterer: cluster.New(0)}
	f := fetcher.NewWithWriter(cfg, collector, os.Stderr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	go func() {
		<-sigChan
		fmt.Fprintf(os.Stderr, "\nReceived interrupt signal, shutting down gracefully...\n")
		cancel()
	}()

	result, err := f.Fetch(ctx)
	if err != nil {
		var ae *axierr.Error
		if !errors.As(err, &ae) {
			ae = axierr.Runtime("fetch_failed", err.Error())
		}
		return fail(*format, ae)
	}

	clusters := collector.clusterer.Clusters()
	shown := clusters
	if *top > 0 && len(clusters) > *top {
		shown = clusters[:*top]
	}

	if *format == "json" {
		return renderPatternsJSON(result, clusters, shown, *samples)
	}
	return renderPatternsToon(cfg, result, clusters, shown, *samples)
}

// clusterCollector adapts the Clusterer to the writer.Writer interface
// so the fetcher can stream pages straight into it.
type clusterCollector struct {
	clusterer *cluster.Clusterer
}

func (c *clusterCollector) WritePage(logs []datadogV2.Log) error {
	for _, log := range logs {
		attrs, ok := log.GetAttributesOk()
		if !ok {
			continue
		}
		var ts time.Time
		if t, ok := attrs.GetTimestampOk(); ok {
			ts = t.UTC()
		}
		c.clusterer.Add(ts, attrs.GetMessage())
	}
	return nil
}

func (c *clusterCollector) Finalize(writer.Meta) error { return nil }
func (c *clusterCollector) Close() error               { return nil }

func renderPatternsToon(cfg *config.Config, result *fetcher.Result, all, shown []*cluster.Cluster, samples bool) int {
	enc := toon.NewEncoder(os.Stdout)

	queryDisplay := cfg.Query
	if queryDisplay == "" {
		queryDisplay = "*"
	}

	if result.Total == 0 {
		enc.Scalar("patterns", fmt.Sprintf("0 logs matched query '%s' in range %s to %s",
			queryDisplay, formatRangeTime(cfg.From), formatRangeTime(cfg.To)))
		enc.List("help", []string{
			"Widen the time range with --from 24h or loosen the query",
		})
		if enc.Err() != nil {
			return exitError
		}
		return exitOK
	}

	enc.Scalar("scanned", result.Total)

	fields := []string{"count", "first_seen", "last_seen", "pattern"}
	if samples {
		fields = append(fields, "sample")
	}
	rows := make([][]any, len(shown))
	for i, cl := range shown {
		row := []any{cl.Count, formatSeen(cl.FirstSeen), formatSeen(cl.LastSeen), cl.Pattern()}
		if samples {
			row = append(row, cl.Sample)
		}
		rows[i] = row
	}
	enc.Table("patterns", fields, rows)

	var help []string
	if len(shown) < len(all) {
		help = append(help, fmt.Sprintf("Showing top %d of %d patterns; rerun with --top 0 for all", len(shown), len(all)))
	}
	if result.NextCursor != "" {
		help = append(help, fmt.Sprintf("Scanned the first %d matching logs; raise --limit to scan more", result.Total))
	}
	help = append(help,
		fmt.Sprintf("Drill into one pattern: dogfetch fetch --query '%s \"<literal text from pattern>\"' --limit 20", queryDisplay))
	if !samples {
		help = append(help, "Add --samples to include one raw example per pattern")
	}
	enc.List("help", help)

	if enc.Err() != nil {
		return exitError
	}
	return exitOK
}

// formatSeen renders a cluster timestamp; logs without timestamps
// leave it empty (unlike range bounds, zero does not mean "now").
func formatSeen(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

type patternRow struct {
	Count     int64  `json:"count"`
	FirstSeen string `json:"first_seen"`
	LastSeen  string `json:"last_seen"`
	Pattern   string `json:"pattern"`
	Sample    string `json:"sample,omitempty"`
}

func renderPatternsJSON(result *fetcher.Result, all, shown []*cluster.Cluster, samples bool) int {
	rows := make([]patternRow, len(shown))
	for i, cl := range shown {
		rows[i] = patternRow{
			Count:     cl.Count,
			FirstSeen: formatSeen(cl.FirstSeen),
			LastSeen:  formatSeen(cl.LastSeen),
			Pattern:   cl.Pattern(),
		}
		if samples {
			rows[i].Sample = cl.Sample
		}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(map[string]any{
		"scanned":        result.Total,
		"total_patterns": len(all),
		"patterns":       rows,
	}); err != nil {
		return exitError
	}
	return exitOK
}
