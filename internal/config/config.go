package config

import (
	"fmt"
	"math"
	"os"
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
	Format     string // "toon", "json", or "ndjson"
	Append     bool
	Fields     []string // projection fields; empty = format default

	// Datadog credentials
	APIKey string
	AppKey string
	Site   string
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

	if c.Format != "json" && c.Format != "ndjson" && c.Format != "toon" {
		return fmt.Errorf("format must be 'toon', 'json', or 'ndjson', got '%s'", c.Format)
	}

	if c.Append && c.Format != "ndjson" {
		return fmt.Errorf("--append only works with --format ndjson")
	}

	// A resumed page in a buffered json document would silently produce
	// a partial export; streaming (ndjson) and context-window (toon)
	// outputs resume fine.
	if c.Cursor != "" && c.Format == "json" {
		return fmt.Errorf("--cursor does not work with --format json; use ndjson or toon")
	}

	return c.ValidateRange()
}

// ValidateRange checks that the resolved time range runs forwards. It
// is the one usage check the aggregate commands share with fetch, so
// they call it directly rather than the whole of ValidateUsage.
func (c *Config) ValidateRange() error {
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

// validSites is the exhaustive list of Datadog site domains. It is an
// allowlist, not a shape check: the API and app keys are attached to
// every request, so accepting any well-formed-looking hostname (as a
// regex would) lets a planted or stale DD_SITE silently redirect
// credential-bearing requests to an arbitrary host under attacker
// control. See https://docs.datadoghq.com/getting_started/site/.
var validSites = map[string]bool{
	"datadoghq.com":     true, // US1
	"us3.datadoghq.com": true, // US3
	"us5.datadoghq.com": true, // US5
	"datadoghq.eu":      true, // EU
	"ddog-gov.com":      true, // US1-FED
	"ap1.datadoghq.com": true, // AP1
	"ap2.datadoghq.com": true, // AP2
}

// ValidateSite rejects a DD_SITE that is not one of Datadog's published
// site domains. An empty site is valid (the SDK defaults to
// datadoghq.com).
func ValidateSite(site string) error {
	if site == "" {
		return nil
	}
	if !validSites[site] {
		return fmt.Errorf("invalid DD_SITE %q: expected one of datadoghq.com, us3.datadoghq.com, us5.datadoghq.com, datadoghq.eu, ddog-gov.com, ap1.datadoghq.com, ap2.datadoghq.com", site)
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
		// n * unit must stay inside time.Duration or it wraps, turning
		// a huge "ago" into a time in the future.
		if n > int64(math.MaxInt64)/int64(unit) {
			return time.Time{}, fmt.Errorf("relative time '%s' is out of range", s)
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

// FileMode is the permission mode for files dogfetch writes. Exports
// hold production log data, so they default to 0600 rather than the
// process umask. DOGFETCH_FILE_MODE overrides it with an octal mode;
// 0666 restores the umask default. An unparseable value falls back to
// 0600.
func FileMode() os.FileMode {
	if v := os.Getenv("DOGFETCH_FILE_MODE"); v != "" {
		if m, err := strconv.ParseUint(v, 8, 32); err == nil {
			mode := os.FileMode(m) & 0o777
			if mode != 0 {
				return mode
			}
		}
	}
	return 0o600
}
