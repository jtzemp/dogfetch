# dogfetch AXI-ification: Agent-Optimized CLI + Claude Code Plugin

## Status

- [x] **Phase 1** — CLI core: dispatch, env precedence, exit codes, relative times, `--limit`, auth file (commit `2abdc23`)
- [x] **Phase 2** — TOON output, projection, truncation, structured errors, home view (commit `076a9f3`)
- [x] **Phase 3** — `dogfetch summary` (Aggregate API) (commit `6ec1d73`)
- [x] **Phase 4** — `dogfetch patterns` (drain-style clustering) (commit `dab4f3e`)
- [x] **Phase 5** — Claude Code plugin + binary wrapper (commit `8e5d2ce`)
- [x] **Phase 6** — Hardening & polish (commit `ec12d87`)

## Context

dogfetch is a single-command Go CLI that exports Datadog logs as JSON/NDJSON. Raw Datadog `Log` objects are huge nested blobs — fine for file export, terrible for agent context windows. Goal: make dogfetch agent-optimized per the AXI principles (axi.md — vendored at `.agents/skills/axi/SKILL.md`) and distribute it as an installable Claude Code plugin from this repo, with auto-downloaded binaries for linux-amd64 and darwin-arm64 (goreleaser already builds all platforms).

Token-saving levers, in order of impact: **field projection** (4 default fields vs full blobs) > **pattern clustering** (collapse repetitive logs) > **aggregates** (answer "how many errors" without raw logs) > **TOON encoding** (~40–60% vs JSON on tabular data). Runtime lever: `--limit` (stop fetching early) and the Aggregate API (no pagination at all for summaries).

User decisions: comprehensive plan, executed in phases. `--format toon` flag + `DOGFETCH_*` env vars for defaulting settings (flag > env > default). Binary install fully automatic (sha256-verified download from GitHub Releases); Datadog auth is unavoidably a one-time manual key creation, so first-run UX guides it (keys readable from `~/.config/dogfetch/env`).

## Design decisions

- **D1 — stdlib `flag` + hand-rolled subcommand dispatch** (no cobra). Verbs: `fetch` (default), `summary`, `patterns`, `auth`, `version`. Back-compat shim: if `os.Args[1]` starts with `-`, treat as implicit `fetch`, so all existing invocations keep working. Bare `dogfetch` (currently an error) becomes the AXI content-first home view.
- **D2 — flag > env > default** via `fs.Visit` to collect explicitly-set flags, then `fs.Set(name, os.Getenv(...))` for unset ones. Mapping: `DOGFETCH_FORMAT`, `DOGFETCH_FIELDS`, `DOGFETCH_LIMIT`, `DOGFETCH_PAGESIZE`, `DOGFETCH_INDEX`. Helper in `internal/cli/env.go`.
- **D3 — In-repo TOON encoder (~150–200 lines), not toon-go.** Our output is exactly TOON's simple subset (scalars, `key[N]{f1,f2}:` tabular blocks). The spec is a fast-moving Working Draft (v3.2); a small golden-tested `internal/toon` package avoids dependency churn. Document the targeted spec version.
- **D4 — Projection in new `internal/project` package**, consumed by writers. Default fields `timestamp,status,service,message`; `--fields` takes attribute paths resolved against `log.GetAttributes()`. Messages truncated (~500 chars) with one AXI hint in the trailing `help[]` block, not per-row. Writer interface keeps `WritePage([]datadogV2.Log)` (fetcher untouched) but `Finalize()` becomes `Finalize(Meta)` where `Meta{Total, NextCursor, Query, Elapsed}`; factory becomes `writer.New(cfg)`.
- **D5 — Structured errors on stdout** (per AXI) via new `internal/axierr` package: `{Code, Message, Help []string, Exit int}`, rendered in the active format. Raw diagnostics/progress stay on stderr/`--errors-out` as today. `cmd.Execute()` returns an int; `main.go` does `os.Exit(cmd.Execute())`. Exit codes: 0 success (incl. empty results), 1 runtime error, 2 usage error. 401/403 render the auth help block with exact Datadog UI URLs.
- **D6 — Default format:** `toon` when writing to stdout, `ndjson` when `--output` is given (file export stays lossless). Breaking change for stdout pipers; `DOGFETCH_FORMAT=ndjson` is the escape hatch — call out in README + release notes.

## Phase 1 — CLI core: dispatch, env precedence, exit codes, relative times, --limit, auth file ✅ DONE

Implemented in commit `2abdc23`:

- `main.go`: `os.Exit(cmd.Execute())`.
- `cmd/root.go`: dispatch table + implicit-fetch shim, `Execute() int`. `cmd/fetch.go` holds the existing flag set + `--limit`, `--fields` (parsed; used in Phase 2).
- `internal/cli/env.go`: `ApplyEnvDefaults(fs, map[flagName]envVar)` (D2).
- `internal/config/config.go`: added `Limit`, `Fields`; `ParseTime` accepts relative durations `^(\d+)([smhdw])$` → `now - d`; `Validate()` split into `ValidateUsage()` (exit 2) and `ValidateAuth()` (exit 1).
- `internal/config/envfile.go`: loads `~/.config/dogfetch/env` (KEY=VALUE, `#` comments, `export `/quote stripping) as fallback when `DD_API_KEY`/`DD_APP_KEY` unset; warns (not fails) on perms looser than 0600. Process env wins.
- `internal/fetcher/fetcher.go`: `--limit` trims the final page to exactly N, stops, returns `*Result{Total, Pages, NextCursor, Elapsed}`; prints "More logs available. Resume with --cursor '…'".

Verified: all tests pass; exit-code matrix confirmed (version→0, bare→2, badflag→2, unknown cmd→2, missing auth→1, bad time→2, neg limit→2, bad env value→2, -h→0); legacy `dogfetch --query … --format ndjson` invocation unchanged.

## Phase 2 — TOON output, projection, truncation, structured errors, home view ✅ DONE

Implemented as planned, with these notes:

- TOON quoting only triggers on `": "` / trailing colon (not bare `:`), so timestamps and URLs stay unquoted.
- `--cursor` now allowed with toon as well as ndjson (only json is rejected).
- Bare `dogfetch` changed from exit 2 to the exit-0 home view (intended; supersedes the Phase 1 matrix entry).
- e2e tests point the SDK at httptest via DD_SITE accepting a full `http(s)://` URL.
- Measured on a 50-log mock with realistic nested attributes: toon stdout is 9,176 bytes vs 33,320 ndjson (~72% reduction). Real-query comparison still worth a spot-check once creds are at hand.

- New `internal/toon/` encoder (D3) with golden tests (`testdata/*.toon`): `Scalar()`, `Table(name, fields, rows)`, quoting/escaping rules.
- New `internal/project/` (D4): projector + attribute-path resolution over typed fields and `AdditionalProperties`.
- New `internal/writer/toon.go`: buffers projected rows (bounded — pairs with `--limit`), `Finalize(meta)` emits `count:` (+ cursor-resume notice), `logs[N]{…}` table, definitive empty state (`logs: 0 matched query "…" in range …`), `help[]` (truncation hint, `--fields` widening, cursor resume).
- `internal/writer/writer.go` + `json.go` + `ndjson.go`: `New(cfg)` factory, `Finalize(Meta)`; json/ndjson apply projector only when `--fields` set.
- New `internal/axierr/` (D5); route fetcher/config errors through it. Default-format switch (D6) in `cmd/fetch.go`.
- New `cmd/home.go`: no-args view — `bin:`, `description:`, `auth: ok (site: …)` / `auth: missing DD_API_KEY`, `help[]` example commands. New `cmd/auth.go`: `dogfetch auth` prints status + remediation (Datadog key URLs, env-file format), exit 0.

**Verify:** golden tests; e2e via `httptest` mock of `/api/v2/logs/events` asserting full stdout bytes for results/empty/truncated/401 + exit codes; manual token comparison toon vs ndjson on a real query (~expect >60% reduction with projection).

## Phase 3 — `dogfetch summary` (Aggregate API — no pagination, fast) ✅ DONE

Implemented as planned, with these notes:

- Three aggregate requests (by status, by service, timeline). Each facet request adds a `__total__` group-by bucket carrying two computes: c0 = total count, c1 = facet cardinality — that's where `total:` and the "top 25 of N" annotation come from (annotation rendered as a `help[]` hint per AXI §9, not inline).
- Timeline interval picked by log-scale nearest to a ~12-bucket target across 1m/5m/1h/1d, whole days beyond.
- `SetUnstableOperationEnabled` removed outright: neither `v2.ListLogsGet` nor `AggregateLogs` is in the SDK's unstable-operation registry, so the call was a no-op.
- help[] suggests `fetch --limit` drill-downs only; the `patterns` suggestion lands with Phase 4 so we never advertise a command that doesn't exist yet.
- summary supports `--format toon|json` (no ndjson — single aggregate object, not a stream).
- Manual totals-vs-Datadog-UI check still pending creds.

- New `cmd/summary.go`, `internal/fetcher/aggregate.go` using SDK `LogsApi.AggregateLogs` (`POST /api/v2/logs/analytics/aggregate`): count grouped by `status` and `service` (limit 25, annotate `(top 25 of N)` when truncated), plus a timeseries bucketed to ~12 intervals (rounded to 1m/5m/1h/1d).
- Output: `total:`, `by_status[…]`, `by_service[…]`, `timeline[…]`, `help[]` suggesting `patterns`/`fetch --limit` drill-downs. Reuse `retry.go`. Fix the misplaced `SetUnstableOperationEnabled` in `client.go` while there.

**Verify:** httptest mock with canned aggregate responses, golden toon output; manual totals vs Datadog UI.

## Phase 4 — `dogfetch patterns` (drain-style clustering) ✅ DONE

Implemented as planned, with these notes:

- Bucket key is **(exact token count, first stable token)** rather than a width-banded count: the 32-token cap already folds long messages into one length class, and exact-length buckets keep merging positional (no alignment heuristics). Merging is classic drain: `<*>` matches anything, differing positions widen to `<*>`, threshold 0.5.
- Masking additions beyond the plan list: numbers with units/punctuation (`500ms`, `95%`, `10:30:00`, `128MiB`), and `key=value` tokens keep the key while masking a volatile value.
- Catch-alls: `(other)` after the 1000-cluster cap, `(empty)` for blank messages; both sort last.
- `--top` (default 50) bounds output rows per AXI minimal-default schemas, with a help hint exposing the full pattern count; `--samples` appends a sample column.
- Fetcher gained `NewWithWriter` so patterns streams pages into the clusterer through the existing writer interface (forced timestamp+message read; no format writer involved).
- summary/home/root help now cross-link patterns (the Phase 3 deferral).

- New `internal/cluster/`: tokenize on whitespace (cap 32 tokens); mask volatile tokens (digits, hex≥8, UUIDs, IPs, quoted values) → `<*>`; bucket by (token-count band, first stable token); merge at ≥0.5 positional similarity; cap 1000 clusters + `(other)` overflow → memory O(clusters), streaming-safe.
- New `cmd/patterns.go`: reuses fetcher with forced timestamp+message projection, default scan cap 10000. Output sorted by count desc: `patterns[N]{count,first_seen,last_seen,pattern}`; `--samples` adds a sample per pattern; `help[]` suggests literal-quoted drill-down queries.

**Verify:** fixture corpora unit tests; shuffle-stability tolerance test.

## Phase 5 — Claude Code plugin + binary wrapper ✅ DONE

Implemented as planned, with these notes:

- Plugin schema re-verified against live docs (June 2026): `.claude-plugin/plugin.json` (name required; explicit semver `version` so users update on release, enforced by a new release.yml step that fails when plugin.json version ≠ tag) and `.claude-plugin/marketplace.json` (owner + plugins[{name, source: "./"}] for same-repo source). plugin.json set to 0.2.0 — next tag must be v0.2.0.
- Wrapper verified against the real v0.1.1 release: resolves latest via GitHub API (24h cache, stale-cache fallback), downloads `dogfetch_<ver>_<Os>_<arch>.tar.gz` + checksums.txt, sha256-verifies, installs atomically (partial-then-rename) to `~/.cache/dogfetch/<ver>/`, execs. Tamper test (poisoned checksums.txt against a local mock server) rejects with the AXI error block, exit 1.
- Version pin file lives at `~/.cache/dogfetch/pin` (written by `--self-update`), not in the plugin dir — plugin dirs are replaced on update.
- `fail()` only runs at top level (never inside `$(...)`) so its stdout error block is never swallowed by command substitution; prerequisite checks (curl/wget, tar, sha256sum/shasum) run upfront for the same reason.
- Wrapper exports `DOGFETCH_FORMAT=toon` unless already set.
- Old hand-rolled SKILL.md instructions in README replaced by the plugin install; skill teaches summary → patterns → fetch --limit order.
- Still manual (needs interactive Claude Code / a Mac): local `/plugin marketplace add jtzemp/dogfetch` smoke test, and a darwin-arm64 wrapper run.

- `.claude-plugin/plugin.json` + `.claude-plugin/marketplace.json` (so `/plugin marketplace add jtzemp/dogfetch` works). **Verify schema against current Claude Code plugin docs at phase start.**
- `skills/dogfetch/SKILL.md`: trigger-shaped description; teaches Datadog query syntax, the agent-optimal call order (**summary → patterns → fetch --limit**), auth setup, invocation via `"${CLAUDE_PLUGIN_ROOT}/scripts/dogfetch.sh"`.
- `scripts/dogfetch.sh` (POSIX sh): map `uname -s/-m` → goreleaser asset names (verified: `dogfetch_<ver>_Darwin_arm64.tar.gz`, `…_Linux_x86_64.tar.gz`); version resolution `DOGFETCH_VERSION` env > pin file > GitHub `releases/latest` (cache resolved tag 24h — anonymous API is 60 req/hr); download archive + `checksums.txt`, verify sha256, extract to `~/.cache/dogfetch/<version>/`, `exec` with args. `--self-update` re-pins latest. AXI-shaped error block on download/verify failure. Wrapper may also export agent-default env (e.g. `DOGFETCH_FORMAT=toon`) unless already set.
- Skill instead of SessionStart hook (dogfetch has no per-repo ambient state; a hook would burn tokens every session). Add plugin-version bump check to `release.yml`. README: install + breaking-change notes.

**Verify:** `bash -n`; run wrapper on a Mac against a real release; tamper-test sha256 rejection; local `/plugin marketplace add` and confirm skill loads and executes end-to-end.

## Phase 6 — Hardening & polish ✅ DONE

Implemented as planned, with these notes:

- `internal/writer/token_regression_test.go`: 50-log fixture with realistic nested attributes; projected TOON vs lossless JSON by len/4 token approximation. Ceiling 0.65×; measured 0.159× (2516 vs 15844 tokens). Runs in CI automatically via `go test ./...`.
- `cmd/exitcode_test.go`: drives the full dispatch path through `Execute()` (sets os.Args) — 15-case 0/1/2 matrix across fetch/summary/patterns/auth/version/home/unknown/help, plus a missing-auth → exit 1 set and a 401 → exit 1 set for all three API subcommands. A combined httptest handler serves both the list and aggregate endpoints. `clearAuth` also clears `XDG_CONFIG_HOME` (not just `HOME`) so the env-file fallback can't leak real creds in CI.
- `make golden-update` (`go test ./internal/toon -update`) and `make lint` (`golangci-lint run`) targets added; `.PHONY` completed for all targets.
- README: added a Commands table (verbs + cheap-first order), `--limit` in the fetch options, a TOON-first Output Formats section, fixed every stale `| jq` stdout example to pass `--format ndjson` (stdout now defaults to TOON), and a release step requiring plugin.json version == tag (the Phase 5 CI gate).
- Whole suite passes under `-race`; lint clean.

Remaining manual items carried from Phase 5 (need interactive Claude Code / a Mac / real creds): live `/plugin marketplace add`, darwin-arm64 wrapper run, and a totals-vs-Datadog-UI spot check. None block tagging v0.2.0.

## Risks

| Risk | Mitigation |
|---|---|
| TOON spec drift | In-repo subset encoder + golden tests (D3) |
| Default stdout format change breaks pipes | ndjson stays default with `--output`; `DOGFETCH_FORMAT` escape hatch; release notes |
| Aggregate API limits/missing facets | Bucket-truncation annotations, retry reuse, definitive empty states |
| Clustering order-sensitivity | Tolerance tests, cluster cap + `(other)` |
| Wrapper ↔ goreleaser asset-name coupling | Single mapping function, integration test vs real release |
| Plugin schema churn | Re-verify plugin.json/marketplace.json at Phase 5 start |
