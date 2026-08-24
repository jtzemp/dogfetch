//go:build testseam

package fetcher

import "os"

// init wires testAPIBaseURLOverride to DOGFETCH_API_URL. This file only
// compiles under the "testseam" build tag (see Makefile's test target),
// so the override — and the ability to redirect credential-bearing
// requests via a process environment variable — does not exist in any
// binary built with plain `go build`.
func init() {
	testAPIBaseURLOverride = func() string { return os.Getenv("DOGFETCH_API_URL") }
}
