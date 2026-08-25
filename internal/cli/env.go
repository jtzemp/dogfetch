// Package cli holds shared helpers for command-line handling.
package cli

import (
	"flag"
	"fmt"
	"os"
)

// ApplyEnvDefaults fills unset flags from environment variables.
// Precedence: explicit flag > environment variable > flag default.
// mapping is flagName -> ENV_VAR. Values are run through the flag's
// own parser, so a bad env value fails the same way a bad flag does.
func ApplyEnvDefaults(fs *flag.FlagSet, mapping map[string]string) error {
	set := make(map[string]bool)
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })

	for name, env := range mapping {
		if set[name] {
			continue
		}
		value, ok := os.LookupEnv(env)
		if !ok || value == "" {
			continue
		}
		if err := fs.Set(name, value); err != nil {
			return fmt.Errorf("invalid %s=%q: %w", env, value, err)
		}
	}
	return nil
}
