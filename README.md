# dogfetch

A simple, efficient CLI tool for fetching logs from Datadog.

## Features

- **Simple query interface** - Fetch logs using Datadog's query syntax
- **Flexible output formats** - JSON or NDJSON (newline-delimited JSON)
- **Memory efficient streaming** - NDJSON mode streams results to disk with minimal memory usage
- **Pagination checkpoint/resume** - Save progress and resume from where you left off if interrupted
- **Configurable time ranges** - Query logs from specific time windows

## Installation

```bash
go install github.com/jtzemp/dogfetch@latest
```

Or build from source:

```bash
git clone https://github.com/jtzemp/dogfetch
cd dogfetch
go build -o dogfetch
```

## Prerequisites

You need a Datadog API key and Application key. Set them as environment variables:

```bash
export DD_API_KEY=your_api_key
export DD_APP_KEY=your_app_key
```

Optionally, set your Datadog site if not using the default (datadoghq.com):

```bash
export DD_SITE=datadoghq.eu
```

## Usage

### Basic Usage

```bash
# Fetch logs matching a query
dogfetch --query 'service:web status:error'

# Specify a custom time range
dogfetch --query 'service:api' --from '2024-01-01T00:00:00Z' --to '2024-01-02T00:00:00Z'

# Save to a specific file
dogfetch --query 'service:database' --output db-logs.json
```

### Command Line Options

```
--query string
    The filter query (search term). Single quote the entire query for best results.
    Example: --query 'service:web status:error'

--index string
    Which index to read from (default "main")

--from string
    Start date/time (default: 24 hours ago)
    Formats: RFC3339 (2024-01-01T00:00:00Z), Unix timestamp (1704067200)

--to string
    End date/time (default: current time)
    Formats: RFC3339 (2024-01-01T00:00:00Z), Unix timestamp (1704067200)

--pageSize int
    How many results to download at a time (default: 1000, max: 5000)

--output string
    Path of file to write results to (default "results.json")

--format string
    Output format: "json" or "ndjson" (default "json")

    json   - Single JSON array, all data loaded into memory
    ndjson - Newline-delimited JSON, streams to disk (low memory)

--cursor string
    Page cursor position for resuming from a specific point
    Only works with streamable formats (ndjson)

--append
    Append to output file instead of overwriting
    Only works with streamable formats (ndjson)
```

### Advanced Usage

#### Streaming Large Datasets

For large log queries, use NDJSON format to minimize memory usage:

```bash
dogfetch --query 'service:api' \
  --format ndjson \
  --output large-export.ndjson \
  --pageSize 5000
```

#### Resume After Interruption

If a large fetch is interrupted, you can resume from where it left off. The cursor value is printed when the fetch stops:

```bash
# First attempt (gets interrupted)
dogfetch --query 'service:web' --format ndjson --output logs.ndjson
# Output: Fetched 50000 logs... cursor: eyJhZnRlciI6eyJpZCI6IjEyMzQ1Njc4OTAiLCJ0aW1lc3RhbXAiOjE3MDQwNjcyMDB9fQ==
# (interrupted)

# Resume from cursor
dogfetch --query 'service:web' \
  --format ndjson \
  --output logs.ndjson \
  --cursor 'eyJhZnRlciI6eyJpZCI6IjEyMzQ1Njc4OTAiLCJ0aW1lc3RhbXAiOjE3MDQwNjcyMDB9fQ==' \
  --append
```

**Why manual checkpointing?** The Datadog SDK provides automatic pagination helpers, but they don't expose the cursor or allow resuming from a specific point. By managing pagination manually, we can print the cursor after each page and allow you to resume long-running fetches if they're interrupted by network issues, rate limits, or system shutdowns. This is particularly useful for large exports that may take hours.

#### Query Multiple Indexes

```bash
dogfetch --query 'status:error' --index 'retention-30'
```

## Output Formats

### JSON (default)

Outputs a single JSON object with all logs in an array:

```json
{
  "logs": [
    {
      "id": "...",
      "attributes": {
        "message": "...",
        "timestamp": "...",
        ...
      }
    },
    ...
  ],
  "meta": {
    "total_fetched": 1523,
    "pages": 2
  }
}
```

### NDJSON (newline-delimited JSON)

Each log is a separate JSON object on its own line:

```json
{"id":"...","attributes":{"message":"...","timestamp":"..."}}
{"id":"...","attributes":{"message":"...","timestamp":"..."}}
```

This format:
- Uses minimal memory (logs are written as they're fetched)
- Can be processed line-by-line with standard tools
- Supports checkpoint/resume with `--cursor` and `--append`

Process with standard tools:
```bash
# Count logs
wc -l logs.ndjson

# Filter with jq
cat logs.ndjson | jq 'select(.attributes.status == "error")'

# Extract specific field
cat logs.ndjson | jq -r '.attributes.message'
```

## Architecture

### Design Goals

1. **Low memory footprint** - Stream data when possible instead of buffering everything
2. **Resilient** - Handle network failures, rate limits, and interruptions gracefully
3. **Fast** - Maximize page size and minimize unnecessary processing
4. **Simple** - Single binary, minimal configuration

### How It Works

```
┌─────────────────┐
│   CLI Parser    │  Parse args, validate config
└────────┬────────┘
         │
┌────────▼────────┐
│  Fetcher Loop   │  Paginate through results with cursor
│                 │  - Retry on transient errors
│                 │  - Print cursor for checkpointing
│                 │  - Handle rate limits
└────────┬────────┘
         │
┌────────▼────────┐
│ Writer Strategy │  JSON: buffer all, write once
│                 │  NDJSON: stream each page
└────────┬────────┘
         │
┌────────▼────────┐
│   Output File   │
└─────────────────┘
```

### Error Handling

- **Transient errors** (network timeouts, 5xx): Exponential backoff retry (3 attempts)
- **Rate limits** (429): Extended backoff based on Retry-After header
- **Permanent errors** (400, 401, 403): Fail immediately with clear message
- **Context cancellation** (Ctrl+C): Graceful shutdown, print current cursor

## Contributing

Contributions welcome! Please open an issue or PR.

## License

MIT

## Acknowledgments

Built with the [Datadog Go API Client](https://github.com/DataDog/datadog-api-client-go).
