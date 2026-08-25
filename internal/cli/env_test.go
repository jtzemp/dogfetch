package cli

import (
	"flag"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestFlagSet() (*flag.FlagSet, *string, *int) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	format := fs.String("format", "ndjson", "")
	limit := fs.Int("limit", 0, "")
	return fs, format, limit
}

func TestApplyEnvDefaultsUsesEnvWhenFlagUnset(t *testing.T) {
	fs, format, limit := newTestFlagSet()
	require.NoError(t, fs.Parse([]string{}))

	t.Setenv("TEST_FORMAT", "json")
	t.Setenv("TEST_LIMIT", "50")

	err := ApplyEnvDefaults(fs, map[string]string{"format": "TEST_FORMAT", "limit": "TEST_LIMIT"})
	require.NoError(t, err)
	assert.Equal(t, "json", *format)
	assert.Equal(t, 50, *limit)
}

func TestApplyEnvDefaultsFlagWins(t *testing.T) {
	fs, format, _ := newTestFlagSet()
	require.NoError(t, fs.Parse([]string{"-format", "ndjson"}))

	t.Setenv("TEST_FORMAT", "json")

	err := ApplyEnvDefaults(fs, map[string]string{"format": "TEST_FORMAT"})
	require.NoError(t, err)
	assert.Equal(t, "ndjson", *format)
}

func TestApplyEnvDefaultsKeepsDefaultWhenEnvUnset(t *testing.T) {
	fs, format, limit := newTestFlagSet()
	require.NoError(t, fs.Parse([]string{}))

	err := ApplyEnvDefaults(fs, map[string]string{"format": "TEST_UNSET_VAR", "limit": "TEST_UNSET_VAR2"})
	require.NoError(t, err)
	assert.Equal(t, "ndjson", *format)
	assert.Equal(t, 0, *limit)
}

func TestApplyEnvDefaultsBadValue(t *testing.T) {
	fs, _, _ := newTestFlagSet()
	require.NoError(t, fs.Parse([]string{}))

	t.Setenv("TEST_LIMIT", "not-a-number")

	err := ApplyEnvDefaults(fs, map[string]string{"limit": "TEST_LIMIT"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "TEST_LIMIT")
}
