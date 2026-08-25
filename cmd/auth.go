package cmd

import (
	"os"

	"github.com/jtzemp/dogfetch/internal/axierr"
	"github.com/jtzemp/dogfetch/internal/config"
	"github.com/jtzemp/dogfetch/internal/toon"
)

// runAuth prints credential status with remediation steps. It always
// exits 0: the status itself is the answer, even when keys are missing.
func runAuth() int {
	creds := config.ResolveCredentials()

	enc := toon.NewEncoder(os.Stdout)
	enc.Scalar("auth", authStatus(creds))
	for _, warning := range creds.Warnings {
		enc.Scalar("warning", warning)
	}

	if creds.APIKey == "" || creds.AppKey == "" {
		enc.List("help", axierr.AuthSetupHelp(creds.Site))
	} else {
		enc.List("help", []string{
			"dogfetch fetch --query 'service:web status:error' --from 2h --limit 100",
		})
	}

	return encStatus(enc)
}
