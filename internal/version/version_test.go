package version

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInfo(t *testing.T) {
	info := Info()

	// Should contain the basic structure
	assert.Contains(t, info, "dogfetch")
	assert.Contains(t, info, "commit:")
	assert.Contains(t, info, "built:")
	assert.Contains(t, info, "go:")
}

func TestShort(t *testing.T) {
	// Save original values
	origVersion := Version
	origCommit := Commit
	defer func() {
		Version = origVersion
		Commit = origCommit
	}()

	t.Run("dev version", func(t *testing.T) {
		Version = "dev"
		Commit = "abc123def456"

		short := Short()
		assert.Contains(t, short, "dev")
		assert.Contains(t, short, "abc123d") // First 7 chars of commit
	})

	t.Run("release version", func(t *testing.T) {
		Version = "1.0.0"
		Commit = "abc123def456"

		short := Short()
		assert.Equal(t, "1.0.0", short)
		assert.NotContains(t, short, "abc") // Should not contain commit
	})
}

func TestDefaultValues(t *testing.T) {
	// When built without ldflags, should have default values
	// We can't test this directly since the values are set at build time,
	// but we can verify the structure is correct

	assert.NotEmpty(t, Version, "Version should not be empty")
	assert.NotEmpty(t, Commit, "Commit should not be empty")
	assert.NotEmpty(t, Date, "Date should not be empty")
}

func TestInfoFormat(t *testing.T) {
	info := Info()

	// Verify the format matches our expected structure
	parts := strings.Split(info, "(")
	assert.Len(t, parts, 2, "Info should have format: 'dogfetch VERSION (details)'")

	// First part should be "dogfetch VERSION"
	assert.True(t, strings.HasPrefix(parts[0], "dogfetch "))

	// Second part should contain commit, built, and go
	details := parts[1]
	assert.Contains(t, details, "commit:")
	assert.Contains(t, details, "built:")
	assert.Contains(t, details, "go:")
}
