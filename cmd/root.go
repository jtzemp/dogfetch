package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/jtzemp/dogfetch/internal/axierr"
	"github.com/jtzemp/dogfetch/internal/version"
)

// Exit codes follow the AXI convention. These alias the axierr
// constants so the process contract has a single definition: fail()
// returns e.Exit, which comes from the same source.
const (
	exitOK    = axierr.ExitOK
	exitError = axierr.ExitError
	exitUsage = axierr.ExitUsage
)

// Execute runs the CLI and returns the process exit code.
func Execute() int {
	args := os.Args[1:]

	if len(args) == 0 {
		// AXI content-first home view: live state, not a usage manual.
		return runHome()
	}

	sub, rest := args[0], args[1:]

	// Back-compat shim: `dogfetch --query ...` is an implicit fetch.
	if strings.HasPrefix(sub, "-") {
		return runFetch(args)
	}

	switch sub {
	case "fetch":
		return runFetch(rest)
	case "summary":
		return runSummary(rest)
	case "patterns":
		return runPatterns(rest)
	case "auth":
		return runAuth()
	case "version":
		fmt.Println(version.Info())
		return exitOK
	default:
		printRootUsage(os.Stderr)
		return fail("toon", axierr.Usage(axierr.UsageCodeUnknownCommand,
			fmt.Sprintf("unknown command %q", sub),
			"dogfetch summary --query 'service:web' --from 2h",
			"dogfetch patterns --query 'service:web' --from 2h",
			"dogfetch fetch --query 'service:web status:error' --limit 100",
			"dogfetch auth",
			"dogfetch version"))
	}
}

func printRootUsage(w *os.File) {
	fmt.Fprintf(w, "dogfetch - Fetch logs from Datadog\n\n")
	fmt.Fprintf(w, "Usage:\n")
	fmt.Fprintf(w, "  dogfetch fetch --query 'service:web status:error'   Fetch logs (default command)\n")
	fmt.Fprintf(w, "  dogfetch summary --query 'service:web' --from 2h    Aggregates: total, by status/service, timeline\n")
	fmt.Fprintf(w, "  dogfetch patterns --query 'service:web' --from 2h   Cluster messages into templates\n")
	fmt.Fprintf(w, "  dogfetch auth                                       Credential status and setup help\n")
	fmt.Fprintf(w, "  dogfetch version                                    Print version information\n\n")
	fmt.Fprintf(w, "Run 'dogfetch fetch --help' for command options.\n")
}
