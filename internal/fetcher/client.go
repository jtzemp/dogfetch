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
	config := datadog.NewConfiguration()
	if site != "" {
		config.SetUnstableOperationEnabled("v2.ListLogsGet", true)
		// Set the server based on site
		config.Servers = datadog.ServerConfigurations{
			{
				URL:         "https://api." + site,
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
