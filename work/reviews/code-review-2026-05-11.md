# Code Review - 2026-05-11

Scope: current repository at `/home/pierre/Tools/nats-docs-mcp-server`.

## Findings

### High: GitHub tokens are never sent to the GitHub API

`internal/fetcher/github.go:211` and `internal/fetcher/github.go:266` create authenticated requests and set `Authorization`/`Accept` headers, but those requests are discarded. Both methods then call `gf.client.Fetch(ctx, url)` at `internal/fetcher/github.go:222` and `internal/fetcher/github.go:277`; `HTTPClient.Fetch` builds a new request with only `User-Agent` at `internal/fetcher/fetcher.go:94`.

Impact: enabling GitHub indexing with a configured PAT still runs unauthenticated. Private repos will not work, and public repo indexing is likely to hit the 60 requests/hour unauthenticated limit before all markdown files are fetched.

Suggested fix: add an `HTTPClient.FetchRequest(*http.Request)` or `FetchWithHeaders` path and use it from the GitHub fetcher. Add a test server assertion that the `Authorization` header is present for tree and contents API calls.

### High: HTML pages wrapped in divs can be indexed with no sections or content

`internal/parser/parser.go:102` treats every `div` as a leaf node: it extracts all descendant text and returns without walking children. If a documentation page has headings inside a wrapper div, for example `<div><h1>Title</h1><p>Body</p></div>`, the parser never sees the heading, `currentSection` stays nil, and no sections are emitted. The server still indexes the page at `internal/server/server.go:269` even when `doc.Sections` is empty.

Impact: real documentation layouts commonly wrap article content in divs. Affected pages will have little or no retrievable body content and weak search behavior, while initialization still reports success because only the page count is checked.

Suggested fix: walk container nodes such as `div` instead of returning early, and only special-case true leaf/content nodes. Add parser coverage for headings nested inside top-level wrapper divs.

### Medium: `max_concurrent` does not actually cap concurrent requests

`internal/fetcher/fetcher.go:43` uses `rate.NewLimiter(rate.Limit(maxConcurrent), maxConcurrent)`, which limits request start rate and burst size, not in-flight concurrency. `FetchAllPages` starts one goroutine per path at `internal/fetcher/fetcher.go:332`, so slow requests can exceed the configured concurrency after the initial burst.

Impact: with slow documentation or GitHub responses, the fetcher can run far more than `max_concurrent` requests simultaneously, increasing load on upstream services and local resource usage.

Suggested fix: use a semaphore or `errgroup.SetLimit` for concurrency, and keep rate limiting separate only if request-per-second throttling is also desired. Add a slow-response test that proves in-flight requests never exceed the configured limit.

### Medium: shipped configuration examples do not match the loader

The loader expects `docs_base_url` (`internal/config/config.go:142`), integer second values for `fetch_timeout` and `synadia.fetch_timeout` (`internal/config/config.go:145` and `internal/config/config.go:175`), and `synadia.*` keys (`internal/config/config.go:169`). The shipped example uses `docs_url` (`config.example.yaml:13`), duration strings like `30s` (`config.example.yaml:18` and `config.example.yaml:70`), and includes `cache_max_age` (`config.example.yaml:42`) even though file loading never reads it. README examples repeat the older `docs_url`/duration style at `README.md:60` and `README.md:85`.

Impact: users following the documented examples either get ignored settings, validation failures, or defaults they did not intend. Cache age from YAML is especially misleading because environment and CLI support exist, but config-file support is missing.

Suggested fix: either update the loader to accept the documented aliases and duration strings, or update all examples/docs to the actual schema. Add a regression test that loads `config.example.yaml`.

### Low: `max_search_results` is validated but never enforced

`MaxSearchResults` is loaded and validated in `internal/config/config.go:154` and `internal/config/config.go:566`, but `handleSearchTool` uses only the request's `limit` with a hard-coded default of 10 at `internal/server/server.go:660`. There is no clamp to `cfg.MaxSearchResults`.

Impact: the advertised maximum does not protect the server from very large result requests, and setting `max_search_results` in config does not affect default search behavior.

Suggested fix: default to `cfg.MaxSearchResults` or clamp `limit` to it, depending on the intended contract. Add a handler test where config max is lower than the requested limit.

## Test And Tooling Notes

- `go test ./...` passes.
- `go vet ./...` passes.
- The test suite currently takes about 49 seconds, mostly because retry/backoff tests sleep for real time. The README says property tests are behind a `property` build tag, but gopter-based tests are currently in normal `*_test.go` files and run during plain `go test ./...`.

## Residual Risk

I did not run `go test -race ./...` during this review because the normal suite already spends significant time in real backoff sleeps. Given the amount of shared mutable state in fetch/index paths, a race run is still worth doing before merging fixes.
