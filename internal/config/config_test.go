package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTime(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantErr   bool
		wantEmpty bool
	}{
		{
			name:      "empty string",
			input:     "",
			wantErr:   false,
			wantEmpty: true,
		},
		{
			name:    "RFC3339 format",
			input:   "2024-01-01T00:00:00Z",
			wantErr: false,
		},
		{
			name:    "Unix timestamp",
			input:   "1704067200",
			wantErr: false,
		},
		{
			name:    "invalid format",
			input:   "not-a-time",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTime(tt.input)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tt.wantEmpty {
				assert.True(t, got.IsZero())
			}
		})
	}
}

func TestParseTimeRFC3339(t *testing.T) {
	input := "2024-01-01T00:00:00Z"
	got, err := ParseTime(input)
	require.NoError(t, err)

	expected := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, got, expected)
}

func TestParseTimeUnix(t *testing.T) {
	got := "1704067200" // 2024-01-01T00:00:00Z
	actualTime, err := ParseTime(got)
	require.NoError(t, err)
	want := time.Unix(1704067200, 0)
	assert.Equal(t, actualTime, want)
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: Config{
				Query:      "service:web",
				Index:      "main",
				PageSize:   1000,
				Format:     "ndjson",
				APIKey:     "test-api-key",
				AppKey:     "test-app-key",
				OutputPath: "",
			},
			wantErr: false,
		},
		{
			name: "missing query",
			config: Config{
				APIKey:   "test-api-key",
				AppKey:   "test-app-key",
				PageSize: 1000,
				Format:   "ndjson",
			},
			wantErr: true,
			errMsg:  "query is required",
		},
		{
			name: "missing API key",
			config: Config{
				Query:    "service:web",
				AppKey:   "test-app-key",
				PageSize: 1000,
				Format:   "ndjson",
			},
			wantErr: true,
			errMsg:  "DD_API_KEY",
		},
		{
			name: "missing App key",
			config: Config{
				Query:    "service:web",
				APIKey:   "test-api-key",
				PageSize: 1000,
				Format:   "ndjson",
			},
			wantErr: true,
			errMsg:  "DD_APP_KEY",
		},
		{
			name: "page size too small",
			config: Config{
				Query:    "service:web",
				APIKey:   "test-api-key",
				AppKey:   "test-app-key",
				PageSize: 0,
				Format:   "ndjson",
			},
			wantErr: true,
			errMsg:  "pageSize must be between",
		},
		{
			name: "page size too large",
			config: Config{
				Query:    "service:web",
				APIKey:   "test-api-key",
				AppKey:   "test-app-key",
				PageSize: 6000,
				Format:   "ndjson",
			},
			wantErr: true,
			errMsg:  "pageSize must be between",
		},
		{
			name: "invalid format",
			config: Config{
				Query:    "service:web",
				APIKey:   "test-api-key",
				AppKey:   "test-app-key",
				PageSize: 1000,
				Format:   "xml",
			},
			wantErr: true,
			errMsg:  "format must be",
		},
		{
			name: "append without ndjson",
			config: Config{
				Query:    "service:web",
				APIKey:   "test-api-key",
				AppKey:   "test-app-key",
				PageSize: 1000,
				Format:   "json",
				Append:   true,
			},
			wantErr: true,
			errMsg:  "--append only works with",
		},
		{
			name: "cursor without ndjson",
			config: Config{
				Query:    "service:web",
				APIKey:   "test-api-key",
				AppKey:   "test-app-key",
				PageSize: 1000,
				Format:   "json",
				Cursor:   "test-cursor",
			},
			wantErr: true,
			errMsg:  "--cursor only works with",
		},
		{
			name: "from after to",
			config: Config{
				Query:    "service:web",
				APIKey:   "test-api-key",
				AppKey:   "test-app-key",
				PageSize: 1000,
				Format:   "ndjson",
				From:     time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				To:       time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			},
			wantErr: true,
			errMsg:  "--from",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg, "error message mismatch")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDefaultFrom(t *testing.T) {
	before := time.Now()
	got := DefaultFrom()
	after := time.Now()

	// Should be approximately 24 hours ago
	expectedBefore := before.Add(-24 * time.Hour)
	expectedAfter := after.Add(-24 * time.Hour)

	assert.False(t, got.Before(expectedBefore.Add(-time.Second)))
	assert.False(t, got.After(expectedAfter.Add(time.Second)))
}
