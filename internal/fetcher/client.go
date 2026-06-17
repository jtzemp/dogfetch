package fetcher

import (
	"context"
	"os"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
)

// Client wraps the Datadog API client
type Client struct {
	api    *datadogV2.LogsApi
	apiKey string
	appKey string
}

// NewClient creates a new Datadog client
func NewClient(apiKey, appKey, site string) *Client {
	// Neither ListLogsGet nor AggregateLogs is in the SDK's unstable
	// operations registry, so no SetUnstableOperationEnabled needed.
	config := datadog.NewConfiguration()
	if base := apiBaseURL(site); base != "" {
		config.Servers = datadog.ServerConfigurations{
			{
				URL:         base,
				Description: "Datadog site",
			},
		}
	}

	apiClient := datadog.NewAPIClient(config)

	return &Client{
		api:    datadogV2.NewLogsApi(apiClient),
		apiKey: apiKey,
		appKey: appKey,
	}
}

// apiBaseURL resolves the Datadog API server URL.
//
// In normal use the site is a Datadog domain (validated upstream by
// config.ValidateSite) and the URL is https://api.<site>; an empty site
// falls through to the SDK default (datadoghq.com).
//
// DOGFETCH_API_URL is an internal override used only by tests to point
// the client at a mock server. It is intentionally read from the process
// environment ONLY — never from the credential env file — so a planted
// ~/.config/dogfetch/env can never redirect credential-bearing requests
// to an arbitrary host. Because it bypasses site validation it must not
// be documented as a user-facing setting.
func apiBaseURL(site string) string {
	if override := os.Getenv("DOGFETCH_API_URL"); override != "" {
		return override
	}
	if site == "" {
		return ""
	}
	return "https://api." + site
}

// GetAPI returns the underlying Logs API
func (c *Client) GetAPI() *datadogV2.LogsApi {
	return c.api
}

// GetContext returns a context with API keys
func (c *Client) GetContext(ctx context.Context) context.Context {
	return context.WithValue(
		ctx,
		datadog.ContextAPIKeys,
		map[string]datadog.APIKey{
			"apiKeyAuth": {Key: c.apiKey},
			"appKeyAuth": {Key: c.appKey},
		},
	)
}
