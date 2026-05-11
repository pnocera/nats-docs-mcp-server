# NATS Documentation Tools

Standalone tools that provide programmatic access to NATS documentation from https://docs.nats.io/. The primary interface is the MCP-independent `nats-docs` CLI, with an MCP server available for MCP clients.

## Features

- **Standalone `nats-docs` CLI** for searching and retrieving NATS docs without MCP
- **Single executable CLI distribution** with the NATS docs archive embedded by default
- **MCP-compliant server** exposing documentation search and retrieval tools
- **Dual documentation sources** - NATS documentation (always enabled) and optional Synadia Control Plane documentation
- **Intelligent query classification** - Automatically routes queries to appropriate documentation source based on keywords
- **Fast in-memory indexing** with TF-IDF relevance ranking and separate indices per source
- **Session-based caching** - documentation fetched once at startup, cached for the session
- **Graceful degradation** - If Syncp documentation fetch fails, server continues with NATS documentation only
- **Single binary distribution** with no external dependencies
- **Cross-platform support** - Linux, macOS, Windows on AMD64 and ARM64
- **Structured logging** with configurable log levels
- **Graceful shutdown** with proper resource cleanup

## Installation

### Download Pre-built Binaries

Download the latest release for your platform from the [releases page](https://github.com/j4ng5y/nats-docs-mcp-server/releases).

Extract the archive:
```bash
tar -xzf nats-docs-mcp-server_*.tar.gz
cd nats-docs-mcp-server_*
```

### Build from Source

Requirements:
- Go 1.24 or later
- GoReleaser (for building)

```bash
# Clone the repository
git clone https://github.com/j4ng5y/nats-docs-mcp-server.git
cd nats-docs-mcp-server

# Build with GoReleaser
goreleaser build --snapshot --clean

# Binaries will be in the dist/ directory
./dist/nats-docs-cli_linux_amd64_v1/nats-docs version
./dist/nats-docs-mcp-server_linux_amd64_v1/nats-docs-mcp-server --version

# Or build the standalone CLI directly
go build -o nats-docs ./cmd/nats-docs-cli
./nats-docs doctor
```

## CLI Usage

`nats-docs` is the primary non-MCP interface. It embeds the downloaded `nats.docs` archive, so the binary can be moved and run as a single executable without a runtime docs zip.

```bash
# Check the embedded archive and index
nats-docs doctor

# Search documentation
nats-docs search "jetstream consumers" --limit 5

# Retrieve a page by document ID or alias
nats-docs get nats-concepts/jetstream/consumers

# List indexed pages
nats-docs list

# Machine-readable output for agents and scripts
nats-docs search "auth callout" --limit 3 --json
nats-docs get running-a-nats-service/configuration/securing_nats/auth_callout --json
```

To inspect another downloaded NATS docs zip instead of the embedded snapshot:

```bash
nats-docs --archive /path/to/nats.docs-master.zip search "leaf nodes"
```

## Configuration

The MCP server can be configured via:
1. Command-line flags (highest priority)
2. Configuration file (YAML)
3. Environment variables
4. Default values (lowest priority)

### Configuration File

Create a `config.yaml` file (see `config.example.yaml` for a complete example):

```yaml
log_level: info
docs_base_url: https://docs.nats.io
fetch_timeout: 30
max_concurrent: 5
max_search_results: 50
nats:
  source_type: archive
  archive_path: assets/nats.docs-master.zip
  archive_url: ""
```

### NATS Documentation Source

By default the server loads NATS documentation from the local `assets/nats.docs-master.zip` archive. This avoids scraping the rendered GitBook site at startup, supports offline initialization, and still returns public `https://docs.nats.io` URLs in search and retrieve results.

The default `archive_path` is relative to the process working directory. Release archives include `assets/nats.docs-master.zip`, so run the binary from the extracted release directory or set `nats.archive_path` / `NATS_DOCS_NATS_ARCHIVE_PATH` to an absolute path. Container users should mount the archive and set `NATS_DOCS_NATS_ARCHIVE_PATH`, or set `NATS_DOCS_NATS_SOURCE_TYPE=site` to use the live-site fallback.

To revert to the previous live-site loader, set `nats.source_type: site`. `nats.archive_url` is reserved for a future remote-download mode and is rejected when archive mode is enabled; use `nats.archive_path` for this release.

### Command-line Flags

```bash
nats-docs-mcp-server --config config.yaml --log-level debug
```

Available flags:
- `--config` - Path to configuration file
- `--log-level` - Log level (debug, info, warn, error)
- `--version` - Display version information
- `--help` - Display help message

### Environment Variables

All configuration options can be set via environment variables with the `NATS_DOCS_` prefix:

```bash
export NATS_DOCS_LOG_LEVEL=debug
export NATS_DOCS_DOCS_BASE_URL=https://docs.nats.io
export NATS_DOCS_FETCH_TIMEOUT=30
export NATS_DOCS_NATS_SOURCE_TYPE=archive
export NATS_DOCS_NATS_ARCHIVE_PATH=assets/nats.docs-master.zip
```

## Caching

The server caches fetched documentation to enable offline operation and faster startup.

### Cache Behavior
- **First archive run**: Reads the local archive and creates cache without network access
- **First site run**: Fetches docs from network, creates cache (~5-30 seconds)
- **Subsequent runs**: Loads from cache if valid, extremely fast (<1 second)
- **Auto-refresh**: Automatically refreshes if cache is older than 7 days (configurable)

### Cache Location
Default: `~/.cache/nats-mcp/`

Override via environment variable:
```bash
export NATS_DOCS_CACHE_DIR=/custom/cache/path
export NATS_DOCS_CACHE_MAX_AGE_DAYS=30  # Default: 7 days
```

### Manual Refresh

Refresh documentation cache on startup:
```bash
./nats-docs-mcp-server --refresh-cache
```

Or use the `refresh_docs_cache` MCP tool from an LLM client to refresh during runtime.

### Offline Mode
The server works completely offline if a valid cache exists. No network connection is required after the initial cache creation.

## Syncp (Synadia Control Plane) Documentation Support

The server supports optional dual documentation sources: NATS and Synadia Control Plane. This feature is disabled by default for backward compatibility.

### Enabling Syncp Support

To enable Syncp documentation support, add the following to your `config.yaml`:

```yaml
syncp:
  enabled: true
  base_url: https://docs.synadia.com/control-plane
  fetch_timeout: 30

classification:
  syncp_keywords:
    - syncp
    - control-plane
    - synadia
    - namespace
    - managed
  nats_keywords:
    - jetstream
    - nats-server
    - nats-cli
    - subject
    - stream
    - consumer
```

### Query Classification

When Syncp is enabled, queries are automatically classified and routed to the appropriate documentation source(s):

| Query Type | Example | Behavior |
|-----------|---------|----------|
| NATS-specific | "jetstream consumer" | Searches NATS documentation only |
| Syncp-specific | "control plane setup" | Searches Syncp documentation only |
| Ambiguous | "authentication" | Searches both sources and merges results |

**Classification Rules:**
- **NATS Only**: Query contains only NATS keywords (e.g., "jetstream", "consumer")
- **Syncp Only**: Query contains only Syncp keywords (e.g., "control-plane", "namespace")
- **Both Sources**: Query contains keywords from both sources or no specific keywords
- Results from both sources are merged and ranked by relevance score

### Graceful Degradation

If Syncp documentation fetch fails during startup:
- The server logs a warning
- Continues operating with NATS documentation only
- No service disruption or error to the user
- This ensures the server remains available even if Syncp source is temporarily unavailable

### Backward Compatibility

- Syncp support is **disabled by default** (`syncp.enabled: false`)
- Existing configurations work unchanged without adding Syncp configuration
- The default NATS-only behavior is preserved
- No breaking changes to the MCP tool interface

## MCP Server Usage

### Running the Server

```bash
./nats-docs-mcp-server --config config.yaml
```

The server communicates via stdio using the MCP protocol. It's designed to be used by MCP clients (like Claude Desktop, IDEs, or other AI assistants).

### MCP Tools

The server exposes two MCP tools:

#### 1. search_nats_docs

Search NATS documentation by query string.

**Parameters:**
- `query` (string, required) - Search query
- `limit` (integer, optional) - Maximum number of results (default: 10)

**Example:**
```json
{
  "query": "jetstream consumer",
  "limit": 5
}
```

**Returns:**
Array of search results with:
- `title` - Document title
- `url` - Document URL
- `source` - Documentation source ("NATS" or "Syncp") when dual sources enabled
- `summary` - Brief excerpt with query context
- `relevance` - Relevance score (0-1)

#### 2. retrieve_nats_doc

Retrieve full content of a specific documentation page.

**Parameters:**
- `doc_id` (string, required) - Document ID or URL path

**Example:**
```json
{
  "doc_id": "/nats-concepts/jetstream"
}
```

**Returns:**
Complete document with:
- `title` - Document title
- `url` - Document URL
- `content` - Full document content
- `sections` - Array of section headings

### Using with Claude Desktop

Add to your Claude Desktop MCP configuration (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS):

```json
{
  "mcpServers": {
    "nats-docs": {
      "command": "/path/to/nats-docs-mcp-server",
      "args": ["--config", "/path/to/config.yaml"]
    }
  }
}
```

## Transport Types

The server supports multiple transport mechanisms for different deployment scenarios:

### STDIO Transport (Default)

The default transport using standard input/output, ideal for local process-based integrations.

**Use Cases:**
- Local development and testing
- Claude Desktop and other local MCP clients
- Embedded integrations where a subprocess manages I/O
- Scenarios where the client and server run on the same machine

**Configuration:**

Via command line (using default):
```bash
./nats-docs-mcp-server --config config.yaml
```

Via environment variable:
```bash
export NATS_DOCS_TRANSPORT_TYPE=stdio
./nats-docs-mcp-server
```

Via config file:
```yaml
transport_type: stdio
```

### SSE Transport (Server-Sent Events)

HTTP-based transport using Server-Sent Events for real-time server-to-client communication.

**Use Cases:**
- Web-based clients
- Browser integrations
- Remote deployments
- Multi-client scenarios with server events

**Configuration:**

Via command line:
```bash
./nats-docs-mcp-server --transport sse --host localhost --port 8080
```

Via environment variables:
```bash
export NATS_DOCS_TRANSPORT_TYPE=sse
export NATS_DOCS_HOST=0.0.0.0
export NATS_DOCS_PORT=8080
./nats-docs-mcp-server
```

Via config file:
```yaml
transport_type: sse
host: 0.0.0.0
port: 8080
```

### StreamableHTTP Transport

Full HTTP transport with request/response and SSE support for enterprise deployments.

**Use Cases:**
- Enterprise integrations
- Full-featured HTTP API requirements
- Load-balanced deployments
- Complex routing scenarios

**Configuration:**

Via command line:
```bash
./nats-docs-mcp-server --transport streamablehttp --host 0.0.0.0 --port 8080
```

Via environment variables:
```bash
export NATS_DOCS_TRANSPORT_TYPE=streamablehttp
export NATS_DOCS_HOST=0.0.0.0
export NATS_DOCS_PORT=8080
./nats-docs-mcp-server
```

Via config file:
```yaml
transport_type: streamablehttp
host: 0.0.0.0
port: 8080
```

## Architecture

### Components

- **MultiSourceFetcher** - HTTP client supporting dual documentation sources (NATS and Syncp) with shared retry logic and rate limiting
- **Parser** - HTML parser extracting structured content from documentation pages (source-agnostic)
- **Index Manager** - Manages separate in-memory TF-IDF search indices for NATS and Syncp documentation
- **Classifier** - Keyword-based query classifier routing queries to appropriate documentation source(s)
- **Search Orchestrator** - Coordinates multi-source searches based on classification and merges results
- **Server** - MCP server core handling protocol communication and tool invocation
- **Tools** - MCP tool handlers for search and retrieval with optional source metadata

### Caching Strategy

The server uses session-based in-memory caching:
- Documentation is fetched once at server startup
- Cache persists for the entire server session
- Cache is NOT persisted to disk between sessions
- Each server restart fetches fresh documentation
- Zero network requests after initial startup

**Benefits:**
- Always fresh documentation (fetched at startup)
- No stale data concerns (cache cleared on restart)
- Fast response times (no network I/O after startup)
- Predictable memory usage (~15-75 MB)

**Trade-offs:**
- Startup time includes documentation fetch (5-30 seconds)
- Requires network connection at startup

## Development

### Running Tests

```bash
# Run unit tests
go test -v ./...

# Run tests with coverage
go test -v -race -coverprofile=coverage.out ./...

# Run specific package tests
go test -v ./internal/index

# Run property-based tests (manual only, takes longer)
go test -v -tags=property ./...

# Run MCP real-world smoke tests against the actual stdio server process
# Writes request/response artifacts under outputs/real_world_tests/<timestamp>/
go run ./scripts/real_world_tests
```

**Note:** Property-based tests are behind a build tag and must be run explicitly with `-tags=property`. They are not run automatically in CI to keep build times fast. To run property tests in GitHub Actions, manually trigger the "Property-Based Tests" workflow from the Actions tab.

### Building

Use GoReleaser for release artifacts and direct `go build` when you only need the standalone CLI:

```bash
# Single executable CLI
go build -o nats-docs ./cmd/nats-docs-cli

# Development build
goreleaser build --snapshot --clean

# Test full release process locally
goreleaser release --snapshot --clean
```

### Project Structure

```
.
├── assets/              # Embedded NATS docs archive
├── cmd/
│   ├── nats-docs-cli/   # Standalone CLI entry point
│   └── server/          # MCP server entry point
├── internal/
│   ├── classifier/      # Query classification (NATS/Syncp routing)
│   ├── config/          # Configuration management
│   ├── fetcher/         # Documentation fetching (dual-source support)
│   ├── parser/          # HTML parsing
│   ├── index/           # Search indexing and management
│   ├── natsdocs/        # MCP-independent NATS docs archive indexing
│   ├── search/          # Multi-source search orchestration
│   ├── logger/          # Structured logging
│   └── server/          # MCP server core
├── skill/               # Agent SKILL.md for the standalone CLI
├── .github/workflows/   # CI/CD workflows
└── .goreleaser.yaml     # Build configuration
```

## Troubleshooting

### Server fails to start

**Problem:** Server exits immediately or shows connection errors.

**Solution:**
- Check that no other process is using stdio
- Verify configuration file is valid YAML
- Check log output for specific errors

### Documentation fetch fails

**Problem:** Server times out or fails during startup.

**Solution:**
- Verify network connectivity to https://docs.nats.io
- Increase `fetch_timeout` in configuration
- Check firewall/proxy settings

### Search returns no results

**Problem:** Search queries return empty results.

**Solution:**
- Verify documentation was fetched successfully (check logs)
- Try broader search terms
- Check that index was built (look for "indexed N documents" in logs)

### High memory usage

**Problem:** Server uses more memory than expected.

**Solution:**
- This is expected - all documentation is cached in memory
- Typical usage: 15-75 MB depending on documentation size
- Restart server to clear cache and fetch fresh documentation

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development guidelines.

## License

[Add your license here]

## Acknowledgments

- Built with [mcp-go](https://github.com/mark3labs/mcp-go) - MCP SDK for Go
- Uses [zerolog](https://github.com/rs/zerolog) for structured logging
- Documentation from [NATS.io](https://nats.io)
