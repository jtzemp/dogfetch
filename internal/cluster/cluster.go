// Package cluster groups log messages into drain-style templates:
// volatile tokens (numbers, hex ids, UUIDs, IPs, quoted values) are
// masked to <*>, and messages whose remaining shape matches an
// existing template merge into it. Memory is O(clusters), so it is
// safe to stream arbitrarily many logs through it.
package cluster

import (
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	// maxTokens caps tokenization; longer messages cluster on their
	// first maxTokens tokens, which also keeps long stack traces in
	// one length bucket.
	maxTokens = 32

	// mask replaces volatile tokens in templates.
	mask = "<*>"

	// mergeThreshold is the minimum positional similarity for a
	// message to join an existing cluster.
	mergeThreshold = 0.5
)

// Cluster is one message template with its occurrence stats.
type Cluster struct {
	Tokens    []string
	Count     int64
	FirstSeen time.Time
	LastSeen  time.Time
	Sample    string // first raw message that formed the cluster

	pattern string // cached Pattern(), invalidated by merge
}

// Pattern renders the template as a single string. The result is
// cached: sorting compares patterns on every count tie, and the long
// tail of singleton clusters is all ties.
func (c *Cluster) Pattern() string {
	if c.pattern == "" {
		c.pattern = strings.Join(c.Tokens, " ")
	}
	return c.pattern
}

// Clusterer accumulates messages into clusters.
type Clusterer struct {
	maxClusters int
	buckets     map[bucketKey][]*Cluster
	count       int
	other       *Cluster // overflow once maxClusters is reached
	empty       *Cluster // messages with no tokens
}

type bucketKey struct {
	tokenCount  int
	firstStable string
}

// New returns a Clusterer capped at maxClusters templates (1000 when
// maxClusters <= 0).
func New(maxClusters int) *Clusterer {
	if maxClusters <= 0 {
		maxClusters = 1000
	}
	return &Clusterer{
		maxClusters: maxClusters,
		buckets:     make(map[bucketKey][]*Cluster),
	}
}

// Add streams one message into the clusterer.
func (c *Clusterer) Add(ts time.Time, message string) {
	tokens := tokenize(message)
	if len(tokens) == 0 {
		if c.empty == nil {
			c.empty = &Cluster{Tokens: []string{"(empty)"}, Sample: message}
		}
		bump(c.empty, ts)
		return
	}

	key := bucketKey{tokenCount: len(tokens), firstStable: firstStable(tokens)}
	var best *Cluster
	bestSim := 0.0
	for _, cl := range c.buckets[key] {
		if sim := similarity(cl.Tokens, tokens); sim > bestSim {
			best, bestSim = cl, sim
		}
	}

	if best != nil && bestSim >= mergeThreshold {
		merge(best, tokens)
		bump(best, ts)
		return
	}

	if c.count >= c.maxClusters {
		if c.other == nil {
			c.other = &Cluster{Tokens: []string{"(other)"}, Sample: message}
		}
		bump(c.other, ts)
		return
	}

	cl := &Cluster{Tokens: tokens, Sample: message}
	bump(cl, ts)
	c.buckets[key] = append(c.buckets[key], cl)
	c.count++
}

// Clusters returns all clusters sorted by count descending; the
// (other) and (empty) catch-alls, when present, sort last.
func (c *Clusterer) Clusters() []*Cluster {
	var out []*Cluster
	for _, bucket := range c.buckets {
		out = append(out, bucket...)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Pattern() < out[j].Pattern()
	})
	if c.other != nil {
		out = append(out, c.other)
	}
	if c.empty != nil {
		out = append(out, c.empty)
	}
	return out
}

func bump(cl *Cluster, ts time.Time) {
	cl.Count++
	if cl.FirstSeen.IsZero() || ts.Before(cl.FirstSeen) {
		cl.FirstSeen = ts
	}
	if ts.After(cl.LastSeen) {
		cl.LastSeen = ts
	}
}

// merge widens a cluster template: positions that differ become <*>.
func merge(cl *Cluster, tokens []string) {
	for i, t := range cl.Tokens {
		if t != mask && t != tokens[i] {
			cl.Tokens[i] = mask
			cl.pattern = ""
		}
	}
}

// similarity is the fraction of positions that match; the template's
// <*> matches anything. Lengths are equal by bucketing.
func similarity(template, tokens []string) float64 {
	matches := 0
	for i, t := range template {
		if t == mask || t == tokens[i] {
			matches++
		}
	}
	return float64(matches) / float64(len(template))
}

// firstStable returns the first non-masked token, or the mask when
// every token is volatile.
func firstStable(tokens []string) string {
	for _, t := range tokens {
		if t != mask {
			return t
		}
	}
	return mask
}

// tokenize splits on whitespace, masks volatile tokens, and caps the
// token count.
func tokenize(message string) []string {
	fields := strings.Fields(message)
	if len(fields) > maxTokens {
		fields = fields[:maxTokens]
	}
	tokens := make([]string, len(fields))
	for i, f := range fields {
		tokens[i] = maskToken(f)
	}
	return tokens
}

var (
	reNumber = regexp.MustCompile(`^[+-]?\d+([.,:]\d+)*(ms|us|ns|s|m|h|%|[KMGTkmgt]i?[Bb]?)?$`)
	reHex    = regexp.MustCompile(`^(0x)?[0-9a-fA-F]{8,}$`)
	reUUID   = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	reIP     = regexp.MustCompile(`^\d{1,3}(\.\d{1,3}){3}(:\d+)?$`)
)

// maskToken decides whether a token is volatile. Wrapping punctuation
// is ignored for the test but preserved in the template; a key=value
// token keeps its key and masks a volatile value.
func maskToken(token string) string {
	if k, v, found := strings.Cut(token, "="); found && k != "" && v != "" {
		if masked := maskToken(v); masked == mask {
			return k + "=" + mask
		}
		return token
	}

	// Fully quoted tokens are user data regardless of content.
	if len(token) >= 2 {
		if (token[0] == '"' && token[len(token)-1] == '"') ||
			(token[0] == '\'' && token[len(token)-1] == '\'') {
			return mask
		}
	}

	core := strings.Trim(token, `()[]{}<>.,;:!?'"`+"`")
	if core == "" {
		return token
	}
	if isVolatile(core) {
		return mask
	}
	return token
}

func isVolatile(s string) bool {
	return reNumber.MatchString(s) ||
		reUUID.MatchString(s) ||
		reIP.MatchString(s) ||
		reHex.MatchString(s)
}
