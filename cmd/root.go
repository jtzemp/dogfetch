package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/jtzemp/dogfetch/internal/version"
)

// Exit codes follow the AXI convention: 0 success, 1 runtime error, 2 usage error.
const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2
)

// Execute runs the CLI and returns the process exit code.
func Execute() int {
	args := os.Args[1:]

	if len(args) == 0 {
		// Bare invocation: fall through to fetch, which reports the
		// missing --query as a usage error. Replaced by the home view
		// in a later phase.
		return runFetch(args)
	}

	sub, rest := args[0], args[1:]

	// Back-compat shim: `dogfetch --query ...` is an implicit fetch.
	if strings.HasPrefix(sub, "-") {
		return runFetch(args)
	}

	switch sub {
	case "fetch":
		return runFetch(rest)
	case "version":
		fmt.Println(version.Info())
		return exitOK
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", sub)
		printRootUsage(os.Stderr)
		return exitUsage
	}
}

func printRootUsage(w *os.File) {
	fmt.Fprintf(w, "dogfetch - Fetch logs from Datadog\n\n")
	fmt.Fprintf(w, "Usage:\n")
	fmt.Fprintf(w, "  dogfetch fetch --query 'service:web status:error'   Fetch logs (default command)\n")
	fmt.Fprintf(w, "  dogfetch version                                    Print version information\n\n")
	fmt.Fprintf(w, "Run 'dogfetch fetch --help' for command options.\n")
}
