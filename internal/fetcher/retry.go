package fetcher

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/jtzemp/dogfetch/internal/axierr"
)

const (
	maxRetries    = 3
	baseBackoff   = 1 * time.Second
	rateLimitWait = 60 * time.Second
)

// RetryableError wraps an error with retry information
type RetryableError struct {
	Err        error
	Retryable  bool
	RetryAfter time.Duration
}

func (e *RetryableError) Error() string {
	return e.Err.Error()
}

// ClassifyError determines if an error is retryable
func ClassifyError(err error, httpResp *http.Response) *RetryableError {
	if err == nil {
		return nil
	}

	re := &RetryableError{
		Err:       err,
		Retryable: false,
	}

	if httpResp == nil {
		// Network error, likely retryable
		re.Retryable = true
		return re
	}

	switch httpResp.StatusCode {
	case 429: // Rate limit
		re.Retryable = true
		re.RetryAfter = parseRetryAfter(httpResp)
		if re.RetryAfter == 0 {
			re.RetryAfter = rateLimitWait
		}
	case 500, 502, 503, 504: // Server errors
		re.Retryable = true
	case 400, 401, 403, 404: // Client errors
		re.Retryable = false
	default:
		if httpResp.StatusCode >= 500 {
			re.Retryable = true
		}
	}

	return re
}

// parseRetryAfter extracts the Retry-After header value
func parseRetryAfter(resp *http.Response) time.Duration {
	header := resp.Header.Get("Retry-After")
	if header == "" {
		return 0
	}

	// Try parsing as seconds
	if seconds, err := strconv.Atoi(header); err == nil {
		return time.Duration(seconds) * time.Second
	}

	// Try parsing as HTTP date
	if t, err := http.ParseTime(header); err == nil {
		return time.Until(t)
	}

	return 0
}

// ExponentialBackoff calculates backoff duration
func ExponentialBackoff(attempt int) time.Duration {
	backoff := float64(baseBackoff) * math.Pow(2, float64(attempt))
	return time.Duration(backoff)
}

// ShouldRetry determines if an operation should be retried
func ShouldRetry(attempt int, err *RetryableError) (bool, time.Duration) {
	if err == nil || !err.Retryable {
		return false, 0
	}

	if attempt >= maxRetries {
		return false, 0
	}

	if err.RetryAfter > 0 {
		return true, err.RetryAfter
	}

	return true, ExponentialBackoff(attempt)
}

// FormatRetryError translates a failed request into a structured,
// agent-actionable error.
func FormatRetryError(err error, httpResp *http.Response) error {
	if httpResp == nil {
		return axierr.Runtime("network_error", fmt.Sprintf("network error: %v", err),
			"Check connectivity and DD_SITE (current default: datadoghq.com)")
	}

	switch httpResp.StatusCode {
	case 401:
		return axierr.Runtime("auth_failed",
			"authentication failed: Datadog rejected DD_API_KEY/DD_APP_KEY (HTTP 401)",
			axierr.AuthHelp()...)
	case 403:
		return axierr.Runtime("permission_denied",
			"permission denied: the Application key lacks the logs_read_data permission (HTTP 403)",
			axierr.AuthHelp()...)
	case 429:
		return axierr.Runtime("rate_limited",
			"Datadog rate limit exceeded after retries",
			"Wait a minute, then rerun; lower --pageSize or narrow the time range")
	default:
		return axierr.Runtime("api_error",
			fmt.Sprintf("Datadog API error (HTTP %d): %v", httpResp.StatusCode, err))
	}
}

// withRetry drives call until it succeeds, the error is not
// retryable, or the attempt budget runs out. Progress is reported on
// errOut. Both the search and aggregate APIs return (T, *http.Response,
// error), so one loop serves both.
func withRetry[T any](
	ctx context.Context,
	errOut io.Writer,
	call func(context.Context) (T, *http.Response, error),
) (T, *http.Response, error) {
	var resp T
	var httpResp *http.Response
	var err error

	for attempt := 0; ; {
		resp, httpResp, err = call(ctx)

		retryErr := ClassifyError(err, httpResp)
		if retryErr == nil {
			return resp, httpResp, nil
		}

		shouldRetry, backoff := ShouldRetry(attempt, retryErr)
		if !shouldRetry {
			return resp, httpResp, FormatRetryError(err, httpResp)
		}

		attempt++
		fmt.Fprintf(errOut, "Error (attempt %d/%d): %v - retrying in %v...\n", attempt, maxRetries, err, backoff)

		select {
		case <-ctx.Done():
			return resp, httpResp, ctx.Err()
		case <-time.After(backoff):
		}
	}
}
