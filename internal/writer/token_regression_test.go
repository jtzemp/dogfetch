package writer

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"

	"github.com/jtzemp/dogfetch/internal/project"
)

// TestTokenRegression guards the core AXI promise: the default TOON
// stdout view must stay dramatically smaller than the full JSON blob
// it replaces. Tokens are approximated as bytes/4 (the usual rule of
// thumb), and the projected TOON output must come in at or under 0.65x
// the lossless JSON for the same 50 logs. Real-world measurement on
// this fixture is well under that; the ceiling catches regressions
// like accidentally widening the default field set or dropping
// projection.
func TestTokenRegression(t *testing.T) {
	logs := realisticLogs(50)

	var toonBuf bytes.Buffer
	tw := NewTOONWriterWithOutput(&toonBuf, project.New(nil))
	if err := tw.WritePage(logs); err != nil {
		t.Fatal(err)
	}
	if err := tw.Finalize(Meta{Total: len(logs), Query: "service:checkout"}); err != nil {
		t.Fatal(err)
	}

	var jsonBuf bytes.Buffer
	jw, err := NewJSONWriterWithOutput(&jsonBuf, nil) // nil projector = full lossless objects
	if err != nil {
		t.Fatal(err)
	}
	if err := jw.WritePage(logs); err != nil {
		t.Fatal(err)
	}
	if err := jw.Finalize(Meta{Total: len(logs)}); err != nil {
		t.Fatal(err)
	}

	toonTok := approxTokens(toonBuf.Len())
	jsonTok := approxTokens(jsonBuf.Len())
	ratio := float64(toonTok) / float64(jsonTok)

	const ceiling = 0.65
	t.Logf("toon=%d bytes (%d tok), json=%d bytes (%d tok), ratio=%.3f",
		toonBuf.Len(), toonTok, jsonBuf.Len(), jsonTok, ratio)

	if ratio > ceiling {
		t.Errorf("TOON/JSON token ratio %.3f exceeds ceiling %.2f; the default TOON view is no longer paying for itself",
			ratio, ceiling)
	}
}

func approxTokens(byteLen int) int {
	return byteLen / 4
}

// realisticLogs builds n logs shaped like real Datadog payloads: typed
// reserved fields, nested custom attributes, tags, and a long message —
// the nested blob that projection is meant to collapse.
func realisticLogs(n int) []datadogV2.Log {
	logs := make([]datadogV2.Log, n)
	base := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	for i := range n {
		id := fmt.Sprintf("AQAAAYxq%06d", i)
		host := fmt.Sprintf("ip-10-0-%d-%d", i%4, i%250)
		svc := "checkout-api"
		status := "error"
		ts := base.Add(time.Duration(i) * time.Second)
		msg := fmt.Sprintf(
			"failed to process payment intent pi_3MtwBwLkdIwHu7ix%d for user u%d: card_declined (insufficient_funds); retrying with exponential backoff attempt %d of 5",
			i, i, i%5)
		logs[i] = datadogV2.Log{
			Id: &id,
			Attributes: &datadogV2.LogAttributes{
				Host:      &host,
				Message:   &msg,
				Service:   &svc,
				Status:    &status,
				Timestamp: &ts,
				Tags:      []string{"env:prod", "team:payments", fmt.Sprintf("pod:checkout-%d", i%8), "region:us-east-1"},
				Attributes: map[string]any{
					"http": map[string]any{
						"status_code": float64(502),
						"method":      "POST",
						"url_details": map[string]any{"path": "/v1/payment_intents", "queryString": map[string]any{}},
					},
					"duration": float64(145000000 + i),
					"network":  map[string]any{"client": map[string]any{"ip": fmt.Sprintf("203.0.113.%d", i%250), "port": float64(40000 + i)}},
					"error": map[string]any{
						"kind":  "CardError",
						"stack": "stripe.error.CardError: card_declined\n  at process /app/svc/payments.py:412\n  at handler /app/svc/api.py:88\n  at dispatch /app/svc/router.py:204",
					},
					"usr": map[string]any{"id": fmt.Sprintf("u%d", i), "tier": "premium"},
				},
			},
		}
	}
	return logs
}
