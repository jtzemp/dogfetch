package cmd

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	"github.com/jtzemp/dogfetch/internal/axierr"
	"github.com/jtzemp/dogfetch/internal/cluster"
	"github.com/jtzemp/dogfetch/internal/config"
	"github.com/jtzemp/dogfetch/internal/fetcher"
	"github.com/jtzemp/dogfetch/internal/toon"
	"github.com/jtzemp/dogfetch/internal/writer"
)

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

	if code, ok := parseFlags(fs, args, *format); !ok {
		return code
	}

	if code, ok := applyEnvDefaults(fs, commonEnvDefaults, *format); !ok {
		return code
	}

	if aerr := requireTOONOrJSON("patterns", *format); aerr != nil {
		return fail("toon", aerr)
	}
	if *limit < 0 || *top < 0 {
		return fail(*format, axierr.Usage(axierr.UsageCodeUsage, "--limit and --top must be >= 0"))
	}

	cfg, aerr := resolveQueryConfig(*query, *index, *from, *to, os.Stderr)
	if aerr != nil {
		return fail(*format, aerr)
	}
	cfg.PageSize = 1000
	cfg.Limit = *limit

	if err := cfg.ValidateRange(); err != nil {
		return fail(*format, axierr.Usage(axierr.UsageCodeUsage, err.Error()))
	}

	if aerr := validateCredentials(cfg); aerr != nil {
		return fail(*format, aerr)
	}

	collector := &clusterCollector{clusterer: cluster.New(0)}
	f := fetcher.NewWithWriter(cfg, collector, os.Stderr)

	ctx, cancel := interruptContext(os.Stderr)
	defer cancel()

	result, err := f.Fetch(ctx)
	if err != nil {
		return fail(*format, asAXIError(err, "fetch_failed"))
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
		toon.EmptyState(enc, "patterns", "logs", queryDisplay, cfg.From, cfg.To)
		return encStatus(enc)
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

	return encStatus(enc)
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
