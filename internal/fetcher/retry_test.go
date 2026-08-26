package fetcher

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		httpResp       *http.Response
		wantRetryable  bool
		wantRetryAfter time.Duration
	}{
		{
			name:          "nil error",
			err:           nil,
			httpResp:      nil,
			wantRetryable: false,
		},
		{
			name:          "network error",
			err:           errors.New("connection refused"),
			httpResp:      nil,
			wantRetryable: true,
		},
		{
			name:          "rate limit 429",
			err:           errors.New("rate limited"),
			httpResp:      &http.Response{StatusCode: 429, Header: http.Header{}},
			wantRetryable: true,
		},
		{
			name:          "server error 500",
			err:           errors.New("internal server error"),
			httpResp:      &http.Response{StatusCode: 500},
			wantRetryable: true,
		},
		{
			name:          "server error 502",
			err:           errors.New("bad gateway"),
			httpResp:      &http.Response{StatusCode: 502},
			wantRetryable: true,
		},
		{
			name:          "server error 503",
			err:           errors.New("service unavailable"),
			httpResp:      &http.Response{StatusCode: 503},
			wantRetryable: true,
		},
		{
			name:          "server error 504",
			err:           errors.New("gateway timeout"),
			httpResp:      &http.Response{StatusCode: 504},
			wantRetryable: true,
		},
		{
			name:          "client error 400",
			err:           errors.New("bad request"),
			httpResp:      &http.Response{StatusCode: 400},
			wantRetryable: false,
		},
		{
			name:          "client error 401",
			err:           errors.New("unauthorized"),
			httpResp:      &http.Response{StatusCode: 401},
			wantRetryable: false,
		},
		{
			name:          "client error 403",
			err:           errors.New("forbidden"),
			httpResp:      &http.Response{StatusCode: 403},
			wantRetryable: false,
		},
		{
			name:          "client error 404",
			err:           errors.New("not found"),
			httpResp:      &http.Response{StatusCode: 404},
			wantRetryable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyError(tt.err, tt.httpResp)

			if tt.err == nil {
				assert.Nil(t, got)
				return
			}

			require.NotNil(t, got)
			assert.Equal(t, tt.wantRetryable, got.Retryable)
		})
	}
}

func TestParseRetryAfter(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   time.Duration
	}{
		{
			name:   "no header",
			header: "",
			want:   0,
		},
		{
			name:   "seconds format",
			header: "60",
			want:   60 * time.Second,
		},
		{
			name:   "invalid format",
			header: "invalid",
			want:   0,
		},
		{
			// Thirty-one years. Honouring it hangs the CLI and any
			// agent waiting on the CLI, so the header is discarded and
			// the caller falls back to its own backoff.
			name:   "absurd seconds discarded",
			header: "1000000000",
			want:   0,
		},
		{
			name:   "just over the cap discarded",
			header: "301",
			want:   0,
		},
		{
			name:   "at the cap honoured",
			header: "300",
			want:   5 * time.Minute,
		},
		{
			// Would overflow time.Duration on the multiply and come
			// back negative.
			name:   "max int seconds discarded",
			header: "9223372036854775807",
			want:   0,
		},
		{
			name:   "http-date in the past discarded",
			header: "Mon, 02 Jan 2006 15:04:05 GMT",
			want:   0,
		},
		{
			name:   "far-future http-date discarded",
			header: "Fri, 31 Dec 2100 23:59:59 GMT",
			want:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{
				Header: http.Header{},
			}
			if tt.header != "" {
				resp.Header.Set("Retry-After", tt.header)
			}

			got := parseRetryAfter(resp)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExponentialBackoff(t *testing.T) {
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{attempt: 0, want: 1 * time.Second},
		{attempt: 1, want: 2 * time.Second},
		{attempt: 2, want: 4 * time.Second},
		{attempt: 3, want: 8 * time.Second},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := ExponentialBackoff(tt.attempt)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestShouldRetry(t *testing.T) {
	tests := []struct {
		name        string
		attempt     int
		err         *RetryableError
		wantRetry   bool
		wantBackoff bool
	}{
		{
			name:      "nil error",
			attempt:   0,
			err:       nil,
			wantRetry: false,
		},
		{
			name:    "not retryable",
			attempt: 0,
			err: &RetryableError{
				Err:       errors.New("bad request"),
				Retryable: false,
			},
			wantRetry: false,
		},
		{
			name:    "retryable first attempt",
			attempt: 0,
			err: &RetryableError{
				Err:       errors.New("temporary error"),
				Retryable: true,
			},
			wantRetry:   true,
			wantBackoff: true,
		},
		{
			name:    "retryable second attempt",
			attempt: 1,
			err: &RetryableError{
				Err:       errors.New("temporary error"),
				Retryable: true,
			},
			wantRetry:   true,
			wantBackoff: true,
		},
		{
			name:    "max retries exceeded",
			attempt: 3,
			err: &RetryableError{
				Err:       errors.New("temporary error"),
				Retryable: true,
			},
			wantRetry: false,
		},
		{
			name:    "custom retry after",
			attempt: 0,
			err: &RetryableError{
				Err:        errors.New("rate limited"),
				Retryable:  true,
				RetryAfter: 5 * time.Second,
			},
			wantRetry:   true,
			wantBackoff: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRetry, gotBackoff := ShouldRetry(tt.attempt, tt.err)

			assert.Equal(t, tt.wantRetry, gotRetry)

			if tt.wantBackoff {
				assert.Greater(t, gotBackoff, time.Duration(0))
			}
		})
	}
}

func TestFormatRetryError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		httpResp *http.Response
		wantMsg  string
	}{
		{
			name:     "network error",
			err:      errors.New("connection refused"),
			httpResp: nil,
			wantMsg:  "network error",
		},
		{
			name:     "401 unauthorized",
			err:      errors.New("unauthorized"),
			httpResp: &http.Response{StatusCode: 401},
			wantMsg:  "authentication failed",
		},
		{
			name:     "403 forbidden",
			err:      errors.New("forbidden"),
			httpResp: &http.Response{StatusCode: 403},
			wantMsg:  "permission denied",
		},
		{
			name:     "429 rate limit",
			err:      errors.New("too many requests"),
			httpResp: &http.Response{StatusCode: 429},
			wantMsg:  "rate limit exceeded",
		},
		{
			name:     "500 server error",
			err:      errors.New("internal server error"),
			httpResp: &http.Response{StatusCode: 500},
			wantMsg:  "API error (HTTP 500)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatRetryError(tt.err, tt.httpResp, "")
			require.NotNil(t, got)
			assert.Contains(t, got.Error(), tt.wantMsg)
		})
	}
}

func TestRateLimitFallsBackWhenRetryAfterIsAbsurd(t *testing.T) {
	// Discarding the header must not mean waiting zero and hammering
	// Datadog: a 429 still falls back to the rate-limit default.
	resp := &http.Response{StatusCode: 429, Header: http.Header{}}
	resp.Header.Set("Retry-After", "1000000000")

	re := ClassifyError(errors.New("rate limited"), resp)
	require.NotNil(t, re)
	assert.True(t, re.Retryable)
	assert.Equal(t, rateLimitWait, re.RetryAfter)

	shouldRetry, backoff := ShouldRetry(0, re)
	assert.True(t, shouldRetry)
	assert.Equal(t, rateLimitWait, backoff)
	assert.LessOrEqual(t, backoff, maxRetryAfter)
}
