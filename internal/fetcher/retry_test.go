package fetcher

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
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
				if got != nil {
					t.Errorf("ClassifyError() expected nil for nil error")
				}
				return
			}

			if got.Retryable != tt.wantRetryable {
				t.Errorf("ClassifyError().Retryable = %v, want %v", got.Retryable, tt.wantRetryable)
			}
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
			if got != tt.want {
				t.Errorf("parseRetryAfter() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExponentialBackoff(t *testing.T) {
	tests := []struct {
		attempt int
		min     time.Duration
		max     time.Duration
	}{
		{attempt: 0, min: 1 * time.Second, max: 1 * time.Second},
		{attempt: 1, min: 2 * time.Second, max: 2 * time.Second},
		{attempt: 2, min: 4 * time.Second, max: 4 * time.Second},
		{attempt: 3, min: 8 * time.Second, max: 8 * time.Second},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := ExponentialBackoff(tt.attempt)
			if got < tt.min || got > tt.max {
				t.Errorf("ExponentialBackoff(%d) = %v, want between %v and %v", tt.attempt, got, tt.min, tt.max)
			}
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

			if gotRetry != tt.wantRetry {
				t.Errorf("ShouldRetry() retry = %v, want %v", gotRetry, tt.wantRetry)
			}

			if tt.wantBackoff && gotBackoff == 0 {
				t.Errorf("ShouldRetry() backoff = 0, want > 0")
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
			wantMsg:  "API error (status 500)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatRetryError(tt.err, tt.httpResp)
			if got == nil {
				t.Fatal("FormatRetryError() returned nil")
			}

			gotMsg := got.Error()
			if !strings.Contains(gotMsg, tt.wantMsg) {
				t.Errorf("FormatRetryError() = %q, want to contain %q", gotMsg, tt.wantMsg)
			}
		})
	}
}

