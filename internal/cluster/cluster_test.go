package cluster

import (
	"fmt"
	"math/rand"
	"testing"
	"time"
)

var t0 = time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)

func TestMaskToken(t *testing.T) {
	tests := []struct{ in, want string }{
		{"hello", "hello"},
		{"12345", "<*>"},
		{"-42", "<*>"},
		{"3.14", "<*>"},
		{"500ms", "<*>"},
		{"95%", "<*>"},
		{"128MiB", "<*>"},
		{"10:30:00", "<*>"},
		{"deadbeefcafe", "<*>"},
		{"0xDEADBEEF", "<*>"},
		{"a3f8c901-4b2d-4e6f-9a1b-2c3d4e5f6a7b", "<*>"},
		{"10.0.3.117", "<*>"},
		{"10.0.3.117:8080", "<*>"},
		{`"bob"`, "<*>"},
		{"'bob'", "<*>"},
		{"(503)", "<*>"},      // wrapping punctuation stripped for the test
		{"attempt:", "attempt:"},
		{"user=123", "user=<*>"},
		{"user=bob", "user=bob"},
		{"pi_3MtwBwLkdIwHu7ix", "pi_3MtwBwLkdIwHu7ix"}, // mixed alnum id: kept (not pure hex/number)
		{"error,", "error,"},
	}
	for _, tt := range tests {
		if got := maskToken(tt.in); got != tt.want {
			t.Errorf("maskToken(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestClusterFixtureCorpus(t *testing.T) {
	c := New(0)
	for i := range 100 {
		c.Add(t0.Add(time.Duration(i)*time.Second),
			fmt.Sprintf("failed to process payment %d for user u%d: card_declined", i, i))
	}
	for i := range 40 {
		c.Add(t0.Add(time.Duration(i)*time.Minute),
			fmt.Sprintf("connection to 10.0.0.%d:5432 timed out after 30s", i))
	}
	for i := range 7 {
		c.Add(t0, fmt.Sprintf("cache miss for key session-%d", i))
	}

	clusters := c.Clusters()
	if len(clusters) != 3 {
		for _, cl := range clusters {
			t.Logf("cluster: %q count=%d", cl.Pattern(), cl.Count)
		}
		t.Fatalf("got %d clusters, want 3", len(clusters))
	}

	top := clusters[0]
	if top.Count != 100 {
		t.Errorf("top cluster count = %d, want 100", top.Count)
	}
	if top.Pattern() != "failed to process payment <*> for user <*> card_declined" {
		t.Errorf("top pattern = %q", top.Pattern())
	}
	if top.FirstSeen != t0 || top.LastSeen != t0.Add(99*time.Second) {
		t.Errorf("first/last seen = %v / %v", top.FirstSeen, top.LastSeen)
	}
	if clusters[1].Count != 40 || clusters[2].Count != 7 {
		t.Errorf("counts = %d, %d; want 40, 7", clusters[1].Count, clusters[2].Count)
	}
	if clusters[1].Pattern() != "connection to <*> timed out after <*>" {
		t.Errorf("second pattern = %q", clusters[1].Pattern())
	}
}

func TestShuffleStability(t *testing.T) {
	var messages []string
	for i := range 200 {
		messages = append(messages, fmt.Sprintf("GET /api/v1/users/%d returned 200 in %dms", i, i%50))
	}
	for i := range 100 {
		messages = append(messages, fmt.Sprintf("retrying job %d after failure: timeout", i))
	}
	for i := range 50 {
		messages = append(messages, fmt.Sprintf("disk usage at %d%% on host web-%d", 50+i%40, i%8))
	}

	counts := func(msgs []string) map[string]int64 {
		c := New(0)
		for _, m := range msgs {
			c.Add(t0, m)
		}
		out := map[string]int64{}
		for _, cl := range c.Clusters() {
			out[cl.Pattern()] = cl.Count
		}
		return out
	}

	base := counts(messages)

	for seed := range 5 {
		shuffled := append([]string(nil), messages...)
		rand.New(rand.NewSource(int64(seed))).Shuffle(len(shuffled), func(i, j int) {
			shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
		})
		got := counts(shuffled)

		// Tolerance: same total, and every big base cluster (>=50)
		// must exist in the shuffled run with a count within 10%.
		for pattern, n := range base {
			if n < 50 {
				continue
			}
			g, ok := got[pattern]
			if !ok {
				t.Errorf("seed %d: pattern %q missing after shuffle", seed, pattern)
				continue
			}
			if diff := g - n; diff > n/10 || diff < -n/10 {
				t.Errorf("seed %d: pattern %q count %d vs base %d", seed, pattern, g, n)
			}
		}
	}
}

func TestClusterCapOverflow(t *testing.T) {
	c := New(3)
	for i := range 10 {
		// Distinct first stable tokens -> distinct clusters.
		c.Add(t0, fmt.Sprintf("alpha%d beta gamma delta", i))
	}
	clusters := c.Clusters()
	if len(clusters) != 4 {
		t.Fatalf("got %d clusters, want 3 + (other)", len(clusters))
	}
	last := clusters[len(clusters)-1]
	if last.Pattern() != "(other)" || last.Count != 7 {
		t.Errorf("overflow = %q count=%d, want (other) count=7", last.Pattern(), last.Count)
	}
}

func TestEmptyMessages(t *testing.T) {
	c := New(0)
	c.Add(t0, "")
	c.Add(t0, "   ")
	c.Add(t0, "real message here")
	clusters := c.Clusters()
	if len(clusters) != 2 {
		t.Fatalf("got %d clusters, want 2", len(clusters))
	}
	last := clusters[len(clusters)-1]
	if last.Pattern() != "(empty)" || last.Count != 2 {
		t.Errorf("empty cluster = %q count=%d", last.Pattern(), last.Count)
	}
}

func TestLongMessagesCapAt32Tokens(t *testing.T) {
	c := New(0)
	long := ""
	for i := range 60 {
		long += fmt.Sprintf("word%d ", i)
	}
	c.Add(t0, long)
	c.Add(t0, long+" extra trailing tail")
	clusters := c.Clusters()
	if len(clusters) != 1 {
		t.Fatalf("long messages with same 32-token prefix should merge, got %d clusters", len(clusters))
	}
	if got := len(clusters[0].Tokens); got != 32 {
		t.Errorf("token cap = %d, want 32", got)
	}
}
