package fetcher

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	"github.com/jtzemp/dogfetch/internal/config"
)

// facetLimit caps how many group-by buckets each facet returns; the
// cardinality compute reports the true distinct count so output can
// say "top 25 of N".
const facetLimit = 25

// FacetCount is one group-by bucket.
type FacetCount struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

// TimePoint is one timeseries bucket.
type TimePoint struct {
	Time  string `json:"time"`
	Count int64  `json:"count"`
}

// Summary holds pre-computed aggregates for a query: the agent's
// "how many / what kind / when" answers without fetching raw logs.
type Summary struct {
	Total              int64        `json:"total"`
	ByStatus           []FacetCount `json:"by_status"`
	StatusCardinality  int64        `json:"status_cardinality,omitempty"`
	ByService          []FacetCount `json:"by_service"`
	ServiceCardinality int64        `json:"service_cardinality,omitempty"`
	Timeline           []TimePoint  `json:"timeline"`
	Interval           string       `json:"interval"`
}

// Aggregator runs Aggregate API queries (no pagination).
type Aggregator struct {
	client *Client
	config *config.Config
	errOut io.Writer
}

// NewAggregator creates an Aggregator.
func NewAggregator(cfg *config.Config, errOut io.Writer) *Aggregator {
	if errOut == nil {
		errOut = os.Stderr
	}
	return &Aggregator{
		client: NewClient(cfg.APIKey, cfg.AppKey, cfg.Site),
		config: cfg,
		errOut: errOut,
	}
}

// totalKey marks the over-all-records bucket in group-by responses.
const totalKey = "__total__"

// Summarize runs the three aggregate queries (by status, by service,
// timeline) and assembles a Summary.
func (a *Aggregator) Summarize(ctx context.Context) (*Summary, error) {
	s := &Summary{Interval: timeseriesInterval(a.config.From, a.config.To)}

	byStatus, err := a.aggregateWithRetry(ctx, a.facetRequest("status"))
	if err != nil {
		return nil, err
	}
	s.ByStatus, s.Total, s.StatusCardinality = splitFacetBuckets(byStatus, "status")

	byService, err := a.aggregateWithRetry(ctx, a.facetRequest("service"))
	if err != nil {
		return nil, err
	}
	s.ByService, _, s.ServiceCardinality = splitFacetBuckets(byService, "service")

	timeline, err := a.aggregateWithRetry(ctx, a.timelineRequest(s.Interval))
	if err != nil {
		return nil, err
	}
	s.Timeline = timeseriesPoints(timeline)

	return s, nil
}

// filter builds the shared query filter.
func (a *Aggregator) filter() *datadogV2.LogsQueryFilter {
	f := datadogV2.NewLogsQueryFilter()
	if a.config.Query != "" {
		f.SetQuery(a.config.Query)
	}
	if a.config.Index != "" {
		f.SetIndexes([]string{a.config.Index})
	}
	f.SetFrom(fmt.Sprintf("%d", a.config.From.UnixMilli()))
	to := a.config.To
	if to.IsZero() {
		to = time.Now()
	}
	f.SetTo(fmt.Sprintf("%d", to.UnixMilli()))
	return f
}

// facetRequest counts logs grouped by facet (top facetLimit by count,
// descending), plus a __total__ bucket whose computes cover all
// matching records: c0 = total count, c1 = facet cardinality.
func (a *Aggregator) facetRequest(facet string) datadogV2.LogsAggregateRequest {
	count := datadogV2.NewLogsCompute(datadogV2.LOGSAGGREGATIONFUNCTION_COUNT)
	cardinality := datadogV2.NewLogsCompute(datadogV2.LOGSAGGREGATIONFUNCTION_CARDINALITY)
	cardinality.SetMetric(facet)

	groupBy := datadogV2.NewLogsGroupBy(facet)
	groupBy.SetLimit(facetLimit)
	sort := datadogV2.NewLogsAggregateSort()
	sort.SetType(datadogV2.LOGSAGGREGATESORTTYPE_MEASURE)
	sort.SetAggregation(datadogV2.LOGSAGGREGATIONFUNCTION_COUNT)
	sort.SetOrder(datadogV2.LOGSSORTORDER_DESCENDING)
	groupBy.SetSort(*sort)
	total := totalKey
	groupBy.SetTotal(datadogV2.LogsGroupByTotalStringAsLogsGroupByTotal(&total))

	req := datadogV2.NewLogsAggregateRequest()
	req.SetFilter(*a.filter())
	req.SetCompute([]datadogV2.LogsCompute{*count, *cardinality})
	req.SetGroupBy([]datadogV2.LogsGroupBy{*groupBy})
	return *req
}

// timelineRequest counts logs bucketed over time.
func (a *Aggregator) timelineRequest(interval string) datadogV2.LogsAggregateRequest {
	count := datadogV2.NewLogsCompute(datadogV2.LOGSAGGREGATIONFUNCTION_COUNT)
	count.SetType(datadogV2.LOGSCOMPUTETYPE_TIMESERIES)
	count.SetInterval(interval)

	req := datadogV2.NewLogsAggregateRequest()
	req.SetFilter(*a.filter())
	req.SetCompute([]datadogV2.LogsCompute{*count})
	return *req
}

// aggregateWithRetry mirrors fetchPageWithRetry for the Aggregate API.
func (a *Aggregator) aggregateWithRetry(ctx context.Context, req datadogV2.LogsAggregateRequest) (datadogV2.LogsAggregateResponse, error) {
	var resp datadogV2.LogsAggregateResponse
	var httpResp *http.Response
	var err error

	attempt := 0
	for {
		resp, httpResp, err = a.client.GetAPI().AggregateLogs(a.client.GetContext(ctx), req)

		retryErr := ClassifyError(err, httpResp)
		if retryErr == nil {
			return resp, nil
		}

		shouldRetry, backoff := ShouldRetry(attempt, retryErr)
		if !shouldRetry {
			return resp, FormatRetryError(err, httpResp)
		}

		attempt++
		fmt.Fprintf(a.errOut, "Error (attempt %d/%d): %v - retrying in %v...\n", attempt, maxRetries, err, backoff)

		select {
		case <-ctx.Done():
			return resp, ctx.Err()
		case <-time.After(backoff):
		}
	}
}

// splitFacetBuckets separates the __total__ bucket from facet buckets.
// Returns facet counts (API order: count desc), the total count, and
// the facet cardinality (0 when the API omitted it).
func splitFacetBuckets(resp datadogV2.LogsAggregateResponse, facet string) ([]FacetCount, int64, int64) {
	var counts []FacetCount
	var total, cardinality int64

	data, ok := resp.GetDataOk()
	if !ok {
		return counts, 0, 0
	}
	for _, bucket := range data.GetBuckets() {
		value, _ := bucket.GetBy()[facet].(string)
		count := computeNumber(bucket, "c0")
		if value == totalKey {
			total = count
			cardinality = computeNumber(bucket, "c1")
			continue
		}
		counts = append(counts, FacetCount{Value: value, Count: count})
	}
	return counts, total, cardinality
}

// timeseriesPoints flattens the timeline response.
func timeseriesPoints(resp datadogV2.LogsAggregateResponse) []TimePoint {
	data, ok := resp.GetDataOk()
	if !ok {
		return nil
	}
	var points []TimePoint
	for _, bucket := range data.GetBuckets() {
		value, ok := bucket.GetComputes()["c0"]
		if !ok || value.LogsAggregateBucketValueTimeseries == nil {
			continue
		}
		for _, p := range value.LogsAggregateBucketValueTimeseries.Items {
			points = append(points, TimePoint{
				Time:  compactTime(p.GetTime()),
				Count: int64(p.GetValue()),
			})
		}
	}
	return points
}

// computeNumber extracts a numeric compute value from a bucket.
func computeNumber(bucket datadogV2.LogsAggregateBucket, key string) int64 {
	value, ok := bucket.GetComputes()[key]
	if !ok || value.LogsAggregateBucketValueSingleNumber == nil {
		return 0
	}
	return int64(*value.LogsAggregateBucketValueSingleNumber)
}

// compactTime re-renders an API timestamp as second-precision RFC3339
// (drops the ".000" milliseconds); unparseable values pass through.
func compactTime(s string) string {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC().Format(time.RFC3339)
	}
	return s
}

// timeseriesIntervals are the allowed timeline bucket sizes.
var timeseriesIntervals = []struct {
	label string
	d     time.Duration
}{
	{"1m", time.Minute},
	{"5m", 5 * time.Minute},
	{"1h", time.Hour},
	{"1d", 24 * time.Hour},
}

// timeseriesInterval picks the bucket size that gets the range closest
// to ~12 intervals, on a log scale across 1m/5m/1h/1d (or whole days
// for very long ranges).
func timeseriesInterval(from, to time.Time) string {
	if to.IsZero() {
		to = time.Now()
	}
	target := to.Sub(from) / 12
	if target <= timeseriesIntervals[0].d {
		return timeseriesIntervals[0].label
	}
	last := timeseriesIntervals[len(timeseriesIntervals)-1]
	if target > last.d {
		days := int(math.Round(float64(target) / float64(24*time.Hour)))
		return fmt.Sprintf("%dd", days)
	}
	best := timeseriesIntervals[0]
	bestDist := math.Inf(1)
	for _, iv := range timeseriesIntervals {
		dist := math.Abs(math.Log(float64(target) / float64(iv.d)))
		if dist < bestDist {
			best, bestDist = iv, dist
		}
	}
	return best.label
}
