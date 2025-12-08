package config

import (
	"fmt"
	"strconv"
	"time"
)

// Config holds all configuration for the fetch operation
type Config struct {
	// Query parameters
	Query string
	Index string
	From  time.Time
	To    time.Time

	// Pagination
	PageSize int32
	Cursor   string

	// Output
	OutputPath string
	Format     string // "json" or "ndjson"
	Append     bool

	// Datadog credentials
	APIKey string
	AppKey string
	Site   string
}

// Validate checks the configuration for errors
func (c *Config) Validate() error {
	if c.Query == "" {
		return fmt.Errorf("query is required")
	}

	if c.APIKey == "" {
		return fmt.Errorf("DD_API_KEY environment variable is required")
	}

	if c.AppKey == "" {
		return fmt.Errorf("DD_APP_KEY environment variable is required")
	}

	if c.PageSize < 1 || c.PageSize > 5000 {
		return fmt.Errorf("pageSize must be between 1 and 5000, got %d", c.PageSize)
	}

	if c.Format != "json" && c.Format != "ndjson" {
		return fmt.Errorf("format must be 'json' or 'ndjson', got '%s'", c.Format)
	}

	if c.Append && c.Format != "ndjson" {
		return fmt.Errorf("--append only works with --format ndjson")
	}

	if c.Cursor != "" && c.Format != "ndjson" {
		return fmt.Errorf("--cursor only works with --format ndjson")
	}

	if !c.To.IsZero() && c.From.After(c.To) {
		return fmt.Errorf("--from (%s) must be before --to (%s)", c.From, c.To)
	}

	return nil
}

// ParseTime parses a time string in various formats
// Supports: RFC3339, Unix timestamp (seconds)
func ParseTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}

	// Try RFC3339 first
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}

	// Try Unix timestamp
	if ts, err := strconv.ParseInt(s, 10, 64); err == nil {
		return time.Unix(ts, 0), nil
	}

	return time.Time{}, fmt.Errorf("unable to parse time '%s': expected RFC3339 or Unix timestamp", s)
}

// DefaultFrom returns the default "from" time (24 hours)
func DefaultFrom() time.Time {
	return time.Now().Add(-24 * time.Hour)
}
