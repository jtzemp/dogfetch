//go:build testseam

package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// aggregateHandler answers the three summary requests, keyed off the
// request body: timeseries compute → timeline, otherwise group_by facet.
func aggregateHandler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t.Helper()
		require.Equal(t, "/api/v2/logs/analytics/aggregate", r.URL.Path)

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		var req struct {
			Compute []struct {
				Type string `json:"type"`
			} `json:"compute"`
			GroupBy []struct {
				Facet string `json:"facet"`
			} `json:"group_by"`
		}
		require.NoError(t, json.Unmarshal(body, &req))

		w.Header().Set("Content-Type", "application/json")
		switch {
		case len(req.Compute) > 0 && req.Compute[0].Type == "timeseries":
			_, _ = w.Write([]byte(`{"data":{"buckets":[{"computes":{"c0":[
				{"time":"2026-06-11T10:00:00.000Z","value":2100},
				{"time":"2026-06-11T11:00:00.000Z","value":2423}
			]}}]}}`))
		case len(req.GroupBy) > 0 && req.GroupBy[0].Facet == "status":
			_, _ = w.Write([]byte(`{"data":{"buckets":[
				{"by":{"status":"__total__"},"computes":{"c0":4523,"c1":3}},
				{"by":{"status":"error"},"computes":{"c0":3200}},
				{"by":{"status":"warn"},"computes":{"c0":1000}},
				{"by":{"status":"info"},"computes":{"c0":323}}
			]}}`))
		case len(req.GroupBy) > 0 && req.GroupBy[0].Facet == "service":
			_, _ = w.Write([]byte(`{"data":{"buckets":[
				{"by":{"service":"__total__"},"computes":{"c0":4523,"c1":87}},
				{"by":{"service":"web"},"computes":{"c0":4000}},
				{"by":{"service":"api"},"computes":{"c0":523}}
			]}}`))
		default:
			t.Errorf("unexpected aggregate request: %s", body)
			w.WriteHeader(http.StatusBadRequest)
		}
	}
}

// runSummaryCapture runs runSummary with stdout captured.
func runSummaryCapture(t *testing.T, args ...string) (string, int) {
	t.Helper()
	return captureStdout(t, func() int { return runSummary(args) })
}

func TestE2ESummaryToon(t *testing.T) {
	srv := httptest.NewServer(aggregateHandler(t))
	defer srv.Close()
	setupEnv(t, srv.URL)

	out, code := runSummaryCapture(t,
		"--query", "service:web",
		"--from", "2026-06-11T10:00:00Z", "--to", "2026-06-11T12:00:00Z")
	assert.Equal(t, 0, code)

	want := "total: 4523\n" +
		"by_status[3]{status,count}:\n" +
		"  error,3200\n" +
		"  warn,1000\n" +
		"  info,323\n" +
		"by_service[2]{service,count}:\n" +
		"  web,4000\n" +
		"  api,523\n" +
		"timeline[2]{time,count}:\n" +
		"  2026-06-11T10:00:00Z,2100\n" +
		"  2026-06-11T11:00:00Z,2423\n" +
		"help[3]:\n" +
		"  by_service shows top 2 of 87 services; narrow with --query 'service:web service:<name>'\n" +
		"  Group repetitive logs: dogfetch patterns --query 'service:web status:<status>'\n" +
		"  See raw logs: dogfetch fetch --query 'service:web status:<status>' --limit 50\n"
	assert.Equal(t, want, out)
}

func TestE2ESummaryJSON(t *testing.T) {
	srv := httptest.NewServer(aggregateHandler(t))
	defer srv.Close()
	setupEnv(t, srv.URL)

	out, code := runSummaryCapture(t,
		"--query", "service:web", "--format", "json",
		"--from", "2026-06-11T10:00:00Z", "--to", "2026-06-11T12:00:00Z")
	assert.Equal(t, 0, code)

	var s map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &s))
	assert.Equal(t, float64(4523), s["total"])
	assert.Len(t, s["by_status"], 3)
	assert.Len(t, s["timeline"], 2)
	assert.Equal(t, "5m", s["interval"], "2h range buckets at 5m")
}

func TestE2ESummaryEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"buckets":[]}}`))
	}))
	defer srv.Close()
	setupEnv(t, srv.URL)

	out, code := runSummaryCapture(t,
		"--query", "service:nope",
		"--from", "2026-06-10T00:00:00Z", "--to", "2026-06-11T00:00:00Z")
	assert.Equal(t, 0, code)

	want := "total: 0 matched query 'service:nope' in range 2026-06-10T00:00:00Z to 2026-06-11T00:00:00Z\n" +
		"help[1]:\n" +
		"  Widen the time range with --from 24h or loosen the query\n"
	assert.Equal(t, want, out)
}

func TestE2ESummaryUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errors":["Forbidden"]}`))
	}))
	defer srv.Close()
	setupEnv(t, srv.URL)

	out, code := runSummaryCapture(t, "--query", "service:web")
	assert.Equal(t, 1, code)
	assert.Contains(t, out, "(permission_denied)")
	assert.Contains(t, out, "logs_read_data")
}

func TestE2ESummaryBadFormat(t *testing.T) {
	setupEnv(t, "http://127.0.0.1:0")
	out, code := runSummaryCapture(t, "--format", "ndjson")
	assert.Equal(t, 2, code)
	assert.Contains(t, out, "summary format must be 'toon' or 'json'")
}

// TestE2ESummaryRateLimitSurfacesError guards the retry path: 429s that
// exhaust retries must produce a rate_limited error, not a false
// "total: 0" (ruled out as the cause of the bug below, but worth
// pinning down since it was the first suspect).
func TestE2ESummaryRateLimitSurfacesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"errors":["rate limited"]}`))
	}))
	defer srv.Close()
	setupEnv(t, srv.URL)

	out, code := runSummaryCapture(t, "--query", "service:web", "--from", "24h")
	assert.Equal(t, 1, code)
	assert.Contains(t, out, "(rate_limited)")
	assert.NotContains(t, out, "total: 0")
}

// TestE2ESummaryMissingTotalBucket guards against the real bug: when
// the Aggregate API response omits the __total__ bucket, Total must
// fall back to the (untruncated) timeline sum, and real by_status data
// must render rather than being discarded as a false empty state.
func TestE2ESummaryMissingTotalBucket(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Compute []struct {
				Type string `json:"type"`
			} `json:"compute"`
			GroupBy []struct {
				Facet string `json:"facet"`
			} `json:"group_by"`
		}
		_ = json.Unmarshal(body, &req)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case len(req.Compute) > 0 && req.Compute[0].Type == "timeseries":
			_, _ = w.Write([]byte(`{"data":{"buckets":[{"computes":{"c0":[{"time":"2026-06-11T10:00:00.000Z","value":9421}]}}]}}`))
		case len(req.GroupBy) > 0 && req.GroupBy[0].Facet == "status":
			// No __total__ bucket in this response - only facet buckets.
			_, _ = w.Write([]byte(`{"data":{"buckets":[
				{"by":{"status":"error"},"computes":{"c0":8000}},
				{"by":{"status":"warn"},"computes":{"c0":1421}}
			]}}`))
		default:
			_, _ = w.Write([]byte(`{"data":{"buckets":[{"by":{"service":"web"},"computes":{"c0":9421}}]}}`))
		}
	}))
	defer srv.Close()
	setupEnv(t, srv.URL)

	out, code := runSummaryCapture(t, "--query", "service:nxserver", "--from", "24h")
	assert.Equal(t, 0, code)
	assert.Contains(t, out, "total: 9421")
	assert.Contains(t, out, "error,8000")
	assert.Contains(t, out, "warn,1421")
}
