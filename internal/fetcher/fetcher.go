package fetcher

import (
	"context"
	"fmt"
	"net/http"
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
}

// New creates a new Fetcher
func New(cfg *config.Config) (*Fetcher, error) {
	client := NewClient(cfg.APIKey, cfg.AppKey, cfg.Site)

	w, err := writer.New(cfg.Format, cfg.OutputPath, cfg.Append)
	if err != nil {
		return nil, fmt.Errorf("failed to create writer: %w", err)
	}

	return &Fetcher{
		client: client,
		config: cfg,
		writer: w,
	}, nil
}

// Fetch retrieves logs from Datadog
func (f *Fetcher) Fetch(ctx context.Context) error {
	defer f.writer.Close()

	cursor := f.config.Cursor
	totalLogs := 0
	pageCount := 0
	startTime := time.Now()

	fmt.Printf("Starting fetch with query: %s\n", f.config.Query)
	fmt.Printf("Time range: %s to %s\n", f.config.From.Format(time.RFC3339), formatToTime(f.config.To))
	fmt.Printf("Page size: %d\n", f.config.PageSize)
	fmt.Println()

	for {
		// Check for cancellation
		select {
		case <-ctx.Done():
			fmt.Printf("\nOperation cancelled. Resume with --cursor '%s'\n", cursor)
			return f.writer.Finalize()
		default:
		}

		// Fetch page with retry
		resp, _, err := f.fetchPageWithRetry(ctx, cursor)
		if err != nil {
			return err
		}

		// Write logs
		logs := resp.GetData()
		if err := f.writer.WritePage(logs); err != nil {
			return fmt.Errorf("failed to write page: %w", err)
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
		fmt.Printf("Fetched %d logs (%d pages, %.1f logs/sec)", totalLogs, pageCount, rate)
		if newCursor != "" {
			fmt.Printf(" - cursor: %s", newCursor)
		}
		fmt.Println()

		// Check if we're done
		if newCursor == "" || len(logs) == 0 {
			break
		}

		cursor = newCursor
	}

	fmt.Printf("\nCompleted! Fetched %d logs in %d pages (%.1fs)\n", totalLogs, pageCount, time.Since(startTime).Seconds())

	return f.writer.Finalize()
}

// fetchPageWithRetry fetches a single page with retry logic
func (f *Fetcher) fetchPageWithRetry(ctx context.Context, cursor string) (datadogV2.LogsListResponse, *http.Response, error) {
	var resp datadogV2.LogsListResponse
	var httpResp *http.Response
	var err error

	attempt := 0
	for {
		resp, httpResp, err = f.fetchPage(ctx, cursor)

		retryErr := ClassifyError(err, httpResp)
		if retryErr == nil {
			// Success
			return resp, httpResp, nil
		}

		shouldRetry, backoff := ShouldRetry(attempt, retryErr)
		if !shouldRetry {
			return resp, httpResp, FormatRetryError(err, httpResp)
		}

		attempt++
		fmt.Printf("Error (attempt %d/%d): %v - retrying in %v...\n", attempt, maxRetries, err, backoff)

		select {
		case <-ctx.Done():
			return resp, httpResp, ctx.Err()
		case <-time.After(backoff):
			// Continue to retry
		}
	}
}

// fetchPage fetches a single page from the API
func (f *Fetcher) fetchPage(ctx context.Context, cursor string) (datadogV2.LogsListResponse, *http.Response, error) {
	// Add API keys to context
	ctx = f.client.GetContext(ctx)

	opts := []datadogV2.ListLogsGetOptionalParameters{}

	// Query
	if f.config.Query != "" {
		opts = append(opts, datadogV2.ListLogsGetOptionalParameters{
			FilterQuery: &f.config.Query,
		})
	}

	// Index
	if f.config.Index != "" {
		indexes := []string{f.config.Index}
		opts = append(opts, datadogV2.ListLogsGetOptionalParameters{
			FilterIndexes: &indexes,
		})
	}

	// Time range
	if !f.config.From.IsZero() {
		opts = append(opts, datadogV2.ListLogsGetOptionalParameters{
			FilterFrom: &f.config.From,
		})
	}

	if !f.config.To.IsZero() {
		opts = append(opts, datadogV2.ListLogsGetOptionalParameters{
			FilterTo: &f.config.To,
		})
	}

	// Page size
	opts = append(opts, datadogV2.ListLogsGetOptionalParameters{
		PageLimit: &f.config.PageSize,
	})

	// Cursor
	if cursor != "" {
		opts = append(opts, datadogV2.ListLogsGetOptionalParameters{
			PageCursor: &cursor,
		})
	}

	return f.client.GetAPI().ListLogsGet(ctx, opts...)
}

// formatToTime formats the "to" time for display
func formatToTime(t time.Time) string {
	if t.IsZero() {
		return "now"
	}
	return t.Format(time.RFC3339)
}
