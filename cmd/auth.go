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
		help := axierr.AuthHelp()
		// The last AuthHelp line points back at `dogfetch auth`;
		// here, show the env-file format instead.
		help = append(help[:len(help)-1],
			"Example ~/.config/dogfetch/env:  DD_API_KEY=<key>  DD_APP_KEY=<key>  DD_SITE=datadoghq.com  (one per line)")
		enc.List("help", help)
	} else {
		enc.List("help", []string{
			"dogfetch fetch --query 'service:web status:error' --from 2h --limit 100",
		})
	}

	if enc.Err() != nil {
		return exitError
	}
	return exitOK
}
