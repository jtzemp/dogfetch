package fetcher

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	"github.com/jtzemp/dogfetch/internal/config"
	"github.com/jtzemp/dogfetch/internal/writer"
)

// Fetcher orchestrates the log fetching process
type Fetcher struct {
	client *Client
	config *config.Config
	writer writer.Writer
	errOut io.Writer
}

// New creates a new Fetcher writing through the configured format.
func New(cfg *config.Config, errOut io.Writer) (*Fetcher, error) {
	w, err := writer.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create writer: %w", err)
	}
	return NewWithWriter(cfg, w, errOut), nil
}

// NewWithWriter creates a Fetcher that feeds pages into a custom
// writer (e.g. the patterns clusterer) instead of a format writer.
func NewWithWriter(cfg *config.Config, w writer.Writer, errOut io.Writer) *Fetcher {
	if errOut == nil {
		errOut = os.Stderr
	}
	return &Fetcher{
		client: NewClient(cfg.APIKey, cfg.AppKey, cfg.Site),
		config: cfg,
		writer: w,
		errOut: errOut,
	}
}

// Result summarizes a completed (or interrupted) fetch.
type Result struct {
	Total      int
	NextCursor string // non-empty when more logs are available (limit hit or cancelled)
}

// Fetch retrieves logs from Datadog
func (f *Fetcher) Fetch(ctx context.Context) (*Result, error) {
	defer f.writer.Close()

	cursor := f.config.Cursor
	totalLogs := 0
	pageCount := 0
	startTime := time.Now()

	fmt.Fprintf(f.errOut, "Starting fetch with query: %s\n", f.config.Query)
	fmt.Fprintf(f.errOut, "Time range: %s to %s\n", f.config.From.Format(time.RFC3339), formatToTime(f.config.To))
	fmt.Fprintf(f.errOut, "Page size: %d\n", f.config.PageSize)
	fmt.Fprintf(f.errOut, "\n")

	result := func(nextCursor string) *Result {
		return &Result{Total: totalLogs, NextCursor: nextCursor}
	}

	finalize := func(nextCursor string) error {
		return f.writer.Finalize(writer.Meta{
			Total:      totalLogs,
			NextCursor: nextCursor,
			Query:      f.config.Query,
			From:       f.config.From,
			To:         f.config.To,
		})
	}

	for {
		// Check for cancellation
		select {
		case <-ctx.Done():
			fmt.Fprintf(f.errOut, "\nOperation cancelled. Resume with --cursor '%s'\n", cursor)
			return result(cursor), finalize(cursor)
		default:
		}

		// Fetch page with retry
		resp, _, err := f.fetchPageWithRetry(ctx, cursor)
		if err != nil {
			return result(cursor), err
		}

		// Trim to --limit before writing
		logs := resp.GetData()
		limitHit := false
		if f.config.Limit > 0 && totalLogs+len(logs) >= f.config.Limit {
			logs = logs[:f.config.Limit-totalLogs]
			limitHit = true
		}

		if err := f.writer.WritePage(logs); err != nil {
			return result(cursor), fmt.Errorf("failed to write page: %w", err)
		}

		pageCount++
		totalLogs += len(logs)

		// Update cursor
		newCursor := ""
		if meta, ok := resp.GetMetaOk(); ok {
			if page, ok := meta.GetPageOk(); ok {
				if after, ok := page.GetAfterOk(); ok {
					newCursor = *after
				}
			}
		}

		// Progress update
		elapsed := time.Since(startTime)
		rate := float64(totalLogs) / elapsed.Seconds()
		fmt.Fprintf(f.errOut, "Fetched %d logs (%d pages, %.1f logs/sec)", totalLogs, pageCount, rate)
		if newCursor != "" {
			fmt.Fprintf(f.errOut, " - cursor: %s", newCursor)
		}
		fmt.Fprintf(f.errOut, "\n")

		if limitHit {
			if newCursor != "" {
				fmt.Fprintf(f.errOut, "\nLimit of %d reached. More logs available. Resume with --cursor '%s'\n", f.config.Limit, newCursor)
			}
			fmt.Fprintf(f.errOut, "\nCompleted! Fetched %d logs in %d pages (%.1fs)\n", totalLogs, pageCount, time.Since(startTime).Seconds())
			return result(newCursor), finalize(newCursor)
		}

		// Check if we're done
		if newCursor == "" || len(logs) == 0 {
			break
		}

		cursor = newCursor
	}

	fmt.Fprintf(f.errOut, "\nCompleted! Fetched %d logs in %d pages (%.1fs)\n", totalLogs, pageCount, time.Since(startTime).Seconds())

	return result(""), finalize("")
}

// fetchPageWithRetry fetches a single page with retry logic
func (f *Fetcher) fetchPageWithRetry(ctx context.Context, cursor string) (datadogV2.LogsListResponse, *http.Response, error) {
	return withRetry(ctx, f.errOut, func(ctx context.Context) (datadogV2.LogsListResponse, *http.Response, error) {
		return f.fetchPage(ctx, cursor)
	})
}

// fetchPage fetches a single page from the API
func (f *Fetcher) fetchPage(ctx context.Context, cursor string) (datadogV2.LogsListResponse, *http.Response, error) {
	// Add API keys to context
	ctx = f.client.GetContext(ctx)

	// Build a single optional parameters struct
	opts := datadogV2.ListLogsGetOptionalParameters{}

	// Query
	if f.config.Query != "" {
		opts.FilterQuery = &f.config.Query
	}

	// Index
	if f.config.Index != "" {
		indexes := []string{f.config.Index}
		opts.FilterIndexes = &indexes
	}

	// Time range
	if !f.config.From.IsZero() {
		opts.FilterFrom = &f.config.From
	}

	if !f.config.To.IsZero() {
		opts.FilterTo = &f.config.To
	}

	// Page size
	opts.PageLimit = &f.config.PageSize

	// Cursor
	if cursor != "" {
		opts.PageCursor = &cursor
	}

	return f.client.GetAPI().ListLogsGet(ctx, opts)
}

// formatToTime formats the "to" time for display
func formatToTime(t time.Time) string {
	if t.IsZero() {
		return "now"
	}
	return t.Format(time.RFC3339)
}
