package config

import (
	"fmt"
	"regexp"
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
	Limit    int // 0 = unlimited

	// Output
	OutputPath string
	Format     string // "json" or "ndjson"
	Append     bool
	Fields     []string // projection fields; empty = format default

	// Datadog credentials
	APIKey string
	AppKey string
	Site   string
}

// Validate checks the configuration for errors
func (c *Config) Validate() error {
	if err := c.ValidateUsage(); err != nil {
		return err
	}
	return c.ValidateAuth()
}

// ValidateUsage checks invocation parameters (bad values exit with usage error code 2)
func (c *Config) ValidateUsage() error {
	if c.Query == "" {
		return fmt.Errorf("query is required")
	}

	if c.PageSize < 1 || c.PageSize > 5000 {
		return fmt.Errorf("pageSize must be between 1 and 5000, got %d", c.PageSize)
	}

	if c.Limit < 0 {
		return fmt.Errorf("limit must be >= 0, got %d", c.Limit)
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

// ValidateAuth checks credentials (missing credentials exit with runtime error code 1)
func (c *Config) ValidateAuth() error {
	if c.APIKey == "" {
		return fmt.Errorf("DD_API_KEY environment variable is required")
	}

	if c.AppKey == "" {
		return fmt.Errorf("DD_APP_KEY environment variable is required")
	}

	return nil
}

var relativeTimeRe = regexp.MustCompile(`^(\d+)([smhdw])$`)

// ParseTime parses a time string in various formats
// Supports: RFC3339, Unix timestamp (seconds), and relative durations
// like "15m", "2h", "3d", "1w" (interpreted as that long ago)
func ParseTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}

	// Try relative duration (e.g. "15m" = 15 minutes ago)
	if m := relativeTimeRe.FindStringSubmatch(s); m != nil {
		n, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("unable to parse relative time '%s': %w", s, err)
		}
		var unit time.Duration
		switch m[2] {
		case "s":
			unit = time.Second
		case "m":
			unit = time.Minute
		case "h":
			unit = time.Hour
		case "d":
			unit = 24 * time.Hour
		case "w":
			unit = 7 * 24 * time.Hour
		}
		return time.Now().Add(-time.Duration(n) * unit), nil
	}

	// Try RFC3339
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}

	// Try Unix timestamp
	if ts, err := strconv.ParseInt(s, 10, 64); err == nil {
		return time.Unix(ts, 0), nil
	}

	return time.Time{}, fmt.Errorf("unable to parse time '%s': expected RFC3339, Unix timestamp, or relative duration like 15m/2h/3d", s)
}

// DefaultFrom returns the default "from" time (24 hours)
func DefaultFrom() time.Time {
	return time.Now().Add(-24 * time.Hour)
}
