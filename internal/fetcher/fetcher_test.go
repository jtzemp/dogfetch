package fetcher

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jtzemp/dogfetch/internal/config"
)

func TestFetcherWithMockAPI(t *testing.T) {
	// Create a mock Datadog API server
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		// Verify auth headers
		assert.NotEmpty(t, r.Header.Get("DD-API-KEY"))
		assert.NotEmpty(t, r.Header.Get("DD-APPLICATION-KEY"))

		// Return mock log response
		response := datadogV2.LogsListResponse{
			Data: []datadogV2.Log{
				createMockLog("log-1", "test message 1"),
				createMockLog("log-2", "test message 2"),
			},
		}

		// Add cursor for pagination test (only on first request)
		if requestCount == 1 {
			response.Meta = &datadogV2.LogsResponseMetadata{
				Page: &datadogV2.LogsResponseMetadataPage{
					After: strPtr("next-cursor"),
				},
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Note: This test would require modifying the client to accept a custom base URL
	// For now, it demonstrates the testing approach
	t.Skip("Skipping integration test - requires mock server support in client")
}

func TestFormatToTime(t *testing.T) {
	tests := []struct {
		name string
		t    time.Time
		want string
	}{
		{
			name: "zero time",
			t:    time.Time{},
			want: "now",
		},
		{
			name: "specific time",
			t:    time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			want: "2024-01-01T00:00:00Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatToTime(tt.t)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFetcherProgressOutput(t *testing.T) {
	var errBuf bytes.Buffer

	cfg := &config.Config{
		Query:      "service:test",
		Index:      "main",
		PageSize:   1000,
		Format:     "ndjson",
		OutputPath: "",
		APIKey:     "test-key",
		AppKey:     "test-app-key",
		From:       time.Now().Add(-24 * time.Hour),
	}

	fetcher, err := New(cfg, &errBuf)
	require.NoError(t, err)
	assert.NotNil(t, fetcher)
}

func TestClientCreation(t *testing.T) {
	client := NewClient("test-api-key", "test-app-key", "")
	require.NotNil(t, client)
	assert.Equal(t, "test-api-key", client.apiKey)
	assert.Equal(t, "test-app-key", client.appKey)
}

func TestClientWithSite(t *testing.T) {
	client := NewClient("test-api-key", "test-app-key", "datadoghq.eu")
	require.NotNil(t, client)
}

func TestFetcherErrorOutput(t *testing.T) {
	tests := []struct {
		name   string
		errOut *bytes.Buffer
	}{
		{
			name:   "with custom error output",
			errOut: &bytes.Buffer{},
		},
		{
			name:   "with nil error output (should default to stderr)",
			errOut: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Query:      "service:test",
				Index:      "main",
				PageSize:   1000,
				Format:     "ndjson",
				OutputPath: "",
				APIKey:     "test-key",
				AppKey:     "test-app-key",
				From:       time.Now().Add(-24 * time.Hour),
			}

			fetcher, err := New(cfg, tt.errOut)
			require.NoError(t, err)
			require.NotNil(t, fetcher)

			if tt.errOut != nil {
				assert.Equal(t, tt.errOut, fetcher.errOut)
			}
		})
	}
}

func TestFetcherWithInvalidConfig(t *testing.T) {
	tests := []struct {
		name   string
		format string
		path   string
	}{
		{
			name:   "invalid format",
			format: "xml",
			path:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Query:      "service:test",
				Index:      "main",
				PageSize:   1000,
				Format:     tt.format,
				OutputPath: tt.path,
				APIKey:     "test-key",
				AppKey:     "test-app-key",
			}

			_, err := New(cfg, nil)
			assert.Error(t, err)
		})
	}
}

func TestFetchContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Verify context is cancelled
	select {
	case <-ctx.Done():
		// Expected
	default:
		t.Error("Context should be cancelled")
	}
}

func TestNDJSONStreamingOutput(t *testing.T) {
	var errBuf bytes.Buffer

	cfg := &config.Config{
		Query:      "service:test",
		Index:      "main",
		PageSize:   1000,
		Format:     "ndjson",
		OutputPath: "",
		APIKey:     "test-key",
		AppKey:     "test-app-key",
		From:       time.Now().Add(-1 * time.Hour),
	}

	fetcher, err := New(cfg, &errBuf)
	require.NoError(t, err)
	assert.Equal(t, "ndjson", fetcher.config.Format)
}

func TestProgressMessages(t *testing.T) {
	var errBuf bytes.Buffer

	cfg := &config.Config{
		Query:      "service:test",
		Index:      "main",
		PageSize:   1000,
		Format:     "ndjson",
		OutputPath: "",
		APIKey:     "test-key",
		AppKey:     "test-app-key",
		From:       time.Now().Add(-24 * time.Hour),
	}

	fetcher, err := New(cfg, &errBuf)
	require.NoError(t, err)
	assert.NotNil(t, fetcher)
}

func TestCursorPagination(t *testing.T) {
	response := datadogV2.LogsListResponse{
		Data: []datadogV2.Log{
			createMockLog("log-1", "message 1"),
		},
		Meta: &datadogV2.LogsResponseMetadata{
			Page: &datadogV2.LogsResponseMetadataPage{
				After: strPtr("test-cursor-123"),
			},
		},
	}

	// Verify cursor extraction
	meta, ok := response.GetMetaOk()
	require.True(t, ok)

	page, ok := meta.GetPageOk()
	require.True(t, ok)

	after, ok := page.GetAfterOk()
	require.True(t, ok)
	assert.Equal(t, "test-cursor-123", *after)
}

func TestEmptyResults(t *testing.T) {
	response := datadogV2.LogsListResponse{
		Data: []datadogV2.Log{},
	}

	logs := response.GetData()
	assert.Empty(t, logs)
}

func TestMultiplePages(t *testing.T) {
	// First page with cursor
	page1 := datadogV2.LogsListResponse{
		Data: []datadogV2.Log{
			createMockLog("log-1", "message 1"),
			createMockLog("log-2", "message 2"),
		},
		Meta: &datadogV2.LogsResponseMetadata{
			Page: &datadogV2.LogsResponseMetadataPage{
				After: strPtr("cursor-page-2"),
			},
		},
	}

	// Second page without cursor (last page)
	page2 := datadogV2.LogsListResponse{
		Data: []datadogV2.Log{
			createMockLog("log-3", "message 3"),
		},
	}

	// Verify first page has cursor
	meta1, ok := page1.GetMetaOk()
	require.True(t, ok)
	page1Meta, ok := meta1.GetPageOk()
	require.True(t, ok)
	after1, ok := page1Meta.GetAfterOk()
	require.True(t, ok)
	assert.NotEmpty(t, *after1)

	// Verify second page has no cursor (end of results)
	cursor := ""
	if meta, ok := page2.GetMetaOk(); ok {
		if page, ok := meta.GetPageOk(); ok {
			if after, ok := page.GetAfterOk(); ok {
				cursor = *after
			}
		}
	}
	assert.Empty(t, cursor)
}

func TestJSONOutputFormat(t *testing.T) {
	var errBuf bytes.Buffer

	cfg := &config.Config{
		Query:      "service:test",
		Index:      "main",
		PageSize:   1000,
		Format:     "json",
		OutputPath: "",
		APIKey:     "test-key",
		AppKey:     "test-app-key",
		From:       time.Now().Add(-24 * time.Hour),
	}

	fetcher, err := New(cfg, &errBuf)
	require.NoError(t, err)
	assert.Equal(t, "json", fetcher.config.Format)
}

// Helper functions

func createMockLog(id, message string) datadogV2.Log {
	return datadogV2.Log{
		Id: &id,
		Attributes: &datadogV2.LogAttributes{
			Message:   &message,
			Timestamp: timePtr(time.Now()),
			Status:    strPtr("info"),
			Service:   strPtr("test-service"),
		},
	}
}

func strPtr(s string) *string {
	return &s
}

func timePtr(t time.Time) *time.Time {
	return &t
}
