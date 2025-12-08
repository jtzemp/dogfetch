package fetcher

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"
)

const (
	maxRetries     = 3
	baseBackoff    = 1 * time.Second
	rateLimitWait  = 60 * time.Second
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

// FormatRetryError creates a user-friendly error message
func FormatRetryError(err error, httpResp *http.Response) error {
	if httpResp == nil {
		return fmt.Errorf("network error: %w", err)
	}

	switch httpResp.StatusCode {
	case 401:
		return fmt.Errorf("authentication failed: check DD_API_KEY and DD_APP_KEY")
	case 403:
		return fmt.Errorf("permission denied: check your API key has logs_read_data permission")
	case 429:
		return fmt.Errorf("rate limit exceeded: %w", err)
	default:
		return fmt.Errorf("API error (status %d): %w", httpResp.StatusCode, err)
	}
}
