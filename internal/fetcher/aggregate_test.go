package fetcher

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
)

func TestTimeseriesInterval(t *testing.T) {
	now := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		rng  time.Duration
		want string
	}{
		{"5 minutes", 5 * time.Minute, "1m"},
		{"15 minutes", 15 * time.Minute, "1m"},
		{"1 hour", time.Hour, "5m"},
		{"6 hours", 6 * time.Hour, "1h"}, // 30m target is log-closer to 1h than 5m
		{"24 hours", 24 * time.Hour, "1h"},
		{"3 days", 72 * time.Hour, "1d"}, // 6h target is log-closer to 1d than 1h
		{"14 days", 14 * 24 * time.Hour, "1d"},
		{"60 days", 60 * 24 * time.Hour, "5d"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := timeseriesInterval(now.Add(-tt.rng), now)
			if got != tt.want {
				t.Errorf("range %v: got %s, want %s", tt.rng, got, tt.want)
			}
		})
	}
}

func TestTimeseriesIntervalZeroTo(t *testing.T) {
	// to.IsZero() means "now"; just assert it doesn't panic and
	// returns an allowed label for a 24h-ago from.
	got := timeseriesInterval(time.Now().Add(-24*time.Hour), time.Time{})
	if got != "1h" {
		t.Errorf("24h-to-now: got %s, want 1h", got)
	}
}

// TestSplitFacetBucketsUppercaseTotal pins a real Datadog API behavior:
// requesting the total bucket keyed "__total__" gets back a bucket
// keyed "__TOTAL__" (uppercase). Regression for the bug where an
// exact-case match let that bucket fall through into the facet list
// (and Total stayed 0) instead of being extracted as the total.
func TestSplitFacetBucketsUppercaseTotal(t *testing.T) {
	var resp datadogV2.LogsAggregateResponse
	body := `{"data":{"buckets":[
		{"by":{"status":"__TOTAL__"},"computes":{"c0":234046,"c1":1}},
		{"by":{"status":"info"},"computes":{"c0":234046}}
	]}}`
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	counts, total, cardinality := splitFacetBuckets(resp, "status")

	if total != 234046 {
		t.Errorf("total = %d, want 234046", total)
	}
	if cardinality != 1 {
		t.Errorf("cardinality = %d, want 1", cardinality)
	}
	if len(counts) != 1 || counts[0].Value != "info" || counts[0].Count != 234046 {
		t.Errorf("counts = %+v, want [{info 234046}] (no __TOTAL__ row leaking through)", counts)
	}
}

func TestCompactTime(t *testing.T) {
	tests := []struct{ in, want string }{
		{"2026-06-11T10:00:00.000Z", "2026-06-11T10:00:00Z"},
		{"2026-06-11T10:00:00+02:00", "2026-06-11T08:00:00Z"},
		{"not-a-time", "not-a-time"},
	}
	for _, tt := range tests {
		if got := compactTime(tt.in); got != tt.want {
			t.Errorf("compactTime(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
