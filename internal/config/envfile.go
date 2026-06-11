package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DefaultEnvFilePath returns the default credentials file path,
// ~/.config/dogfetch/env
func DefaultEnvFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "dogfetch", "env"), nil
}

// LoadEnvFile parses a KEY=VALUE file. Blank lines and lines starting
// with # are ignored. A leading "export " and surrounding quotes on
// values are stripped so shell-style env files work as-is.
func LoadEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	vars := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}
		if key != "" {
			vars[key] = value
		}
	}
	return vars, scanner.Err()
}

// Credentials holds resolved Datadog credentials and any non-fatal
// warnings produced while resolving them.
type Credentials struct {
	APIKey   string
	AppKey   string
	Site     string
	Source   string // "env", "file", or "env+file"
	Warnings []string
}

// ResolveCredentials reads DD_API_KEY, DD_APP_KEY, and DD_SITE from the
// process environment, falling back to the env file for any that are
// unset. Process environment always wins.
func ResolveCredentials() Credentials {
	c := Credentials{
		APIKey: os.Getenv("DD_API_KEY"),
		AppKey: os.Getenv("DD_APP_KEY"),
		Site:   os.Getenv("DD_SITE"),
		Source: "env",
	}

	if c.APIKey != "" && c.AppKey != "" && c.Site != "" {
		return c
	}

	path, err := DefaultEnvFilePath()
	if err != nil {
		return c
	}
	info, err := os.Stat(path)
	if err != nil {
		return c // no file; env-only resolution
	}

	vars, err := LoadEnvFile(path)
	if err != nil {
		c.Warnings = append(c.Warnings, fmt.Sprintf("could not read %s: %v", path, err))
		return c
	}

	usedFile := false
	if c.APIKey == "" && vars["DD_API_KEY"] != "" {
		c.APIKey = vars["DD_API_KEY"]
		usedFile = true
	}
	if c.AppKey == "" && vars["DD_APP_KEY"] != "" {
		c.AppKey = vars["DD_APP_KEY"]
		usedFile = true
	}
	if c.Site == "" && vars["DD_SITE"] != "" {
		c.Site = vars["DD_SITE"]
		usedFile = true
	}

	if usedFile {
		if c.APIKey != "" || c.AppKey != "" {
			c.Source = "env+file"
			if os.Getenv("DD_API_KEY") == "" && os.Getenv("DD_APP_KEY") == "" {
				c.Source = "file"
			}
		}
		if mode := info.Mode().Perm(); mode&0o077 != 0 {
			c.Warnings = append(c.Warnings, fmt.Sprintf("%s is readable by other users (mode %04o) - run: chmod 600 %s", path, mode, path))
		}
	}

	return c
}
