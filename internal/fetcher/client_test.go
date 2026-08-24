//go:build testseam

package fetcher

import "testing"

func TestAPIBaseURL(t *testing.T) {
	t.Run("empty site falls through to SDK default", func(t *testing.T) {
		t.Setenv("DOGFETCH_API_URL", "")
		if got := apiBaseURL(""); got != "" {
			t.Errorf("apiBaseURL(\"\") = %q, want empty", got)
		}
	})

	t.Run("domain becomes https://api.<site>", func(t *testing.T) {
		t.Setenv("DOGFETCH_API_URL", "")
		if got := apiBaseURL("datadoghq.eu"); got != "https://api.datadoghq.eu" {
			t.Errorf("got %q, want https://api.datadoghq.eu", got)
		}
	})

	t.Run("DOGFETCH_API_URL override wins", func(t *testing.T) {
		t.Setenv("DOGFETCH_API_URL", "http://127.0.0.1:9999")
		if got := apiBaseURL("datadoghq.com"); got != "http://127.0.0.1:9999" {
			t.Errorf("got %q, want the override", got)
		}
	})
}
