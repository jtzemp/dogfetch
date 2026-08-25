---
name: dogfetch
description: >
  Query Datadog logs: count/summarize errors, find log patterns, fetch raw
  log lines. Use when investigating production issues, checking error rates,
  debugging a service from its logs, or whenever Datadog log data would
  answer the question. Requires DD_API_KEY/DD_APP_KEY (one-time setup).
---

# dogfetch — Datadog logs for agents

Run every command through the wrapper (it auto-downloads and caches a
sha256-verified binary on first use):

```sh
"${CLAUDE_PLUGIN_ROOT}/scripts/dogfetch.sh" <command> [flags]
```

Output is TOON (compact tabular text) by default — read it directly, no
parsing needed. Errors also arrive structured on stdout with a `help[]`
block telling you the fix.

## Call order: cheapest first

1. **`summary`** — counts by status/service plus a timeline, one fast call,
   no raw logs. Answers "how many errors, which service, when did it spike".

   ```sh
   "${CLAUDE_PLUGIN_ROOT}/scripts/dogfetch.sh" summary --query 'service:web' --from 2h
   ```

2. **`patterns`** — collapses repetitive logs into templates with counts
   (`failed to process payment <*> for user <*>`). Answers "what kinds of
   errors are these" without reading thousands of lines.

   ```sh
   "${CLAUDE_PLUGIN_ROOT}/scripts/dogfetch.sh" patterns --query 'service:web status:error' --from 2h
   ```

3. **`fetch --limit N`** — raw log lines, projected to
   timestamp/status/service/message. Only reach for this after summary or
   patterns tells you what to look at, and keep `--limit` small (20-100).

   ```sh
   "${CLAUDE_PLUGIN_ROOT}/scripts/dogfetch.sh" fetch --query 'service:web status:error "card_declined"' --from 2h --limit 50
   ```

Add `--fields timestamp,status,service,message,host,http.status_code` to
widen columns (any Datadog attribute path works). For a full lossless
export use `--output logs.ndjson`.

## Datadog query syntax (the parts that matter)

- `service:web` `status:error` `host:web-1` `env:prod` — facet filters
- `status:(error OR warn)` — OR within a facet
- `service:web -status:info` — exclude with `-`
- `"card_declined"` — quoted literal searches message text
- `@http.status_code:500` — `@` prefix for custom attributes
- Combine with spaces (implicit AND): `service:web status:error "timeout"`

Time flags: `--from 15m` / `2h` / `3d` / RFC3339 / Unix seconds. Default
range is the last 24h.

## Auth (one-time)

If a command exits with an auth error, tell the user to:

1. Create an API key: https://app.datadoghq.com/organization-settings/api-keys
2. Create an Application key: https://app.datadoghq.com/organization-settings/application-keys
3. Store them (either works):
   - `~/.config/dogfetch/env` file with `DD_API_KEY=...` and `DD_APP_KEY=...` lines (chmod 600), or
   - export `DD_API_KEY` / `DD_APP_KEY` in the shell environment
   - non-default site (EU etc.): add `DD_SITE=datadoghq.eu`

Check status anytime:

```sh
"${CLAUDE_PLUGIN_ROOT}/scripts/dogfetch.sh" auth
```

## Maintenance

- `"${CLAUDE_PLUGIN_ROOT}/scripts/dogfetch.sh" --self-update` — pin the newest release
- `DOGFETCH_VERSION=0.2.0` env var forces a specific version
- Bare invocation (no args) prints a live status view with examples
