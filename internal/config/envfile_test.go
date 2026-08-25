package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadEnvFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "env")
	content := `# Datadog credentials
DD_API_KEY=abc123

export DD_APP_KEY="def456"
DD_SITE='datadoghq.eu'
MALFORMED LINE
  SPACED_KEY = spaced value
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	vars, err := LoadEnvFile(path)
	require.NoError(t, err)
	assert.Equal(t, "abc123", vars["DD_API_KEY"])
	assert.Equal(t, "def456", vars["DD_APP_KEY"])
	assert.Equal(t, "datadoghq.eu", vars["DD_SITE"])
	assert.Equal(t, "spaced value", vars["SPACED_KEY"])
	assert.NotContains(t, vars, "MALFORMED LINE")
}

func TestLoadEnvFileMissing(t *testing.T) {
	_, err := LoadEnvFile(filepath.Join(t.TempDir(), "nope"))
	assert.Error(t, err)
}

func TestResolveCredentialsEnvWins(t *testing.T) {
	t.Setenv("DD_API_KEY", "env-api")
	t.Setenv("DD_APP_KEY", "env-app")
	t.Setenv("DD_SITE", "datadoghq.com")

	creds := ResolveCredentials()
	assert.Equal(t, "env-api", creds.APIKey)
	assert.Equal(t, "env-app", creds.AppKey)
	assert.Equal(t, "datadoghq.com", creds.Site)
	assert.Equal(t, "env", creds.Source)
}

func TestResolveCredentialsFileFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // os.UserHomeDir() uses USERPROFILE on Windows
	t.Setenv("DD_API_KEY", "")
	t.Setenv("DD_APP_KEY", "")
	t.Setenv("DD_SITE", "")

	cfgDir := filepath.Join(home, ".config", "dogfetch")
	require.NoError(t, os.MkdirAll(cfgDir, 0o700))
	envFile := filepath.Join(cfgDir, "env")
	require.NoError(t, os.WriteFile(envFile, []byte("DD_API_KEY=file-api\nDD_APP_KEY=file-app\n"), 0o600))

	creds := ResolveCredentials()
	assert.Equal(t, "file-api", creds.APIKey)
	assert.Equal(t, "file-app", creds.AppKey)
	assert.Equal(t, "file", creds.Source)
	assert.Empty(t, creds.Warnings)
}

func TestResolveCredentialsLoosePermsWarn(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file permission bits are not meaningful on Windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("DD_API_KEY", "")
	t.Setenv("DD_APP_KEY", "")
	t.Setenv("DD_SITE", "")

	cfgDir := filepath.Join(home, ".config", "dogfetch")
	require.NoError(t, os.MkdirAll(cfgDir, 0o700))
	envFile := filepath.Join(cfgDir, "env")
	require.NoError(t, os.WriteFile(envFile, []byte("DD_API_KEY=k\nDD_APP_KEY=a\n"), 0o644))

	creds := ResolveCredentials()
	assert.Equal(t, "k", creds.APIKey)
	require.NotEmpty(t, creds.Warnings)
	assert.Contains(t, creds.Warnings[0], "chmod 600")
}
