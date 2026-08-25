# Reproducing the benchmarks

Steps to reproduce the token comparison in the [README's Benchmarks section](../README.md#benchmarks)
against your own Datadog data.

1. **Pick a fixed query and time window** so every tool sees identical data. Use an absolute range, not a
   relative one like `--from 30m`, so it doesn't drift between calls:

   ```bash
   FROM='2024-01-01T00:00:00Z'   # pick your own window
   TO='2024-01-01T00:30:00Z'
   QUERY='service:your-service'
   ```

2. **Capture dogfetch's output** in both formats:

   ```bash
   dogfetch fetch --query "$QUERY" --from "$FROM" --to "$TO" --limit 200 --format toon   > toon.txt
   dogfetch fetch --query "$QUERY" --from "$FROM" --to "$TO" --limit 200 --format ndjson > ndjson.txt
   ```

3. **Capture the Datadog MCP tool's output.** This one can't be scripted from a shell — it only runs inside
   an agent session with [the Datadog MCP server](https://docs.datadoghq.com/mcp_server/setup/) set up
   (`/plugin install datadog@claude-plugins-official` then `/ddsetup`). In that session, ask the agent to call
   `search_datadog_logs` with the same `query`/`from`/`to` and save the raw result to a file — once with
   default settings, once with `max_tokens: 20000` (its hard cap) — e.g.:

   > Call search_datadog_logs with query "service:your-service", from "2024-01-01T00:00:00Z", to
   > "2024-01-01T00:30:00Z", and save the full raw output to mcp-default.txt. Then call it again with
   > max_tokens 20000 and save that to mcp-maxtokens.txt.

4. **Count tokens** the same way for every file:

   ```bash
   pip install tiktoken
   cat > count_tokens.py << 'EOF'
   import sys, tiktoken
   enc = tiktoken.get_encoding("cl100k_base")
   for path in sys.argv[1:]:
       text = open(path, encoding="utf-8", errors="replace").read()
       print(f"{len(enc.encode(text)):>8}  tokens   {path}")
   EOF
   python3 count_tokens.py toon.txt ndjson.txt mcp-default.txt mcp-maxtokens.txt
   ```
