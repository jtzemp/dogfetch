package fetcher

import (
	"context"

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

// testAPIBaseURLOverride lets tests point the client at a mock server
// instead of a real Datadog site. It stays nil in any binary built
// without the "testseam" tag — see testseam.go — so the override code
// is physically absent from release builds and can never be reached via
// a process environment variable in production.
var testAPIBaseURLOverride func() string

// apiBaseURL resolves the Datadog API server URL.
//
// The site is a Datadog domain (validated upstream by
// config.ValidateSite) and the URL is https://api.<site>; an empty site
// falls through to the SDK default (datadoghq.com).
func apiBaseURL(site string) string {
	if testAPIBaseURLOverride != nil {
		if override := testAPIBaseURLOverride(); override != "" {
			return override
		}
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
