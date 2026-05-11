# Proposal: Archive-Backed NATS Docs Ingestion

Date: 2026-05-11

## Summary

Replace the current NATS `docs.nats.io` sitemap plus HTML scraping path with an archive-backed Markdown ingestion path for the NATS documentation source. The server should load the NATS docs from a GitHub zip archive, parse GitBook metadata, parse Markdown files directly, and index the resulting documents.

Recommendation: implement archive ingestion as the new NATS docs path behind a config flag first, keep the existing site/sitemap fetcher as a fallback during parity testing, then make archive ingestion the default once URL and content coverage are validated.

## Archive Analysis

Archive inspected: `/home/pierre/Tools/nats-docs-mcp-server/assets/nats.docs-master.zip`

Observed structure:

- Compressed size: 5.5 MB
- Expanded size reported by `unzip -l`: 24,171,042 bytes
- Total entries: 720
- Files: 588
- Directories: 132
- Markdown files: 287
- Non-empty Markdown files: 261
- Empty Markdown files: 26
- `SUMMARY.md` Markdown links: 186
- `.gitbook.yaml` redirect entries: 227
- Generated HTML files:
  - `docs/`: 160 HTML files
  - `_examples/`: 49 HTML files

Important files:

- `SUMMARY.md`: navigation and canonical page order.
- `.bookignore`: excludes `_book/`, `_docs/`, `_examples/`, `_tools/`, `docs/`, `Makefile`, `building_the_book.md`, `.gitignore`, and `.bookignore`.
- `.gitbook.yaml`: redirect map from old public paths to Markdown targets.
- `book.json`: GitBook metadata and plugins.

Important content categories:

- Active Markdown content is mostly under:
  - `nats-concepts/`
  - `using-nats/`
  - `running-a-nats-service/`
  - `reference/`
  - `release_notes/`
  - root docs such as `README.md`, `overview.md`, and `reference-protocols.md`
- `docs/` is generated website output and should not be indexed as source content.
- `_examples/` is generated example HTML and should not be indexed by default.
- `zh-cn/` contains mostly empty placeholder Markdown files and should be skipped.
- `legacy/` contains non-empty legacy docs. It is mostly referenced through `.gitbook.yaml` redirects, not primary navigation.

The archive uses GitBook-specific Markdown syntax such as `{% hint %}`, `{% tabs %}`, `{% tab title="Go" %}`, `{% embed url="..." %}`, and HTML fragments such as `<figure><img ...>`. The current Goldmark parser will parse normal Markdown well, but should be paired with a lightweight GitBook preprocessor to avoid indexing directive noise and to preserve useful tab/hint/embed text.

## Goals

- Fetch NATS documentation with one archive download instead of hundreds of page requests.
- Parse Markdown source instead of rendered HTML.
- Avoid current HTML parser brittleness around wrapper elements and generated markup.
- Reduce startup variability and upstream load.
- Preserve existing MCP tool behavior:
  - `search_nats_docs`
  - `retrieve_nats_doc`
  - accepted doc IDs with or without leading/trailing slash
- Preserve public `docs.nats.io` URLs in search and retrieve output.
- Support offline startup from cached archive-derived documents.

## Non-Goals

- Do not redesign the search ranking algorithm in this change.
- Do not change the MCP tool interface.
- Do not replace Synadia fetching unless a comparable archive source is added later.
- Do not index generated website output from `docs/` as source-of-truth content.

## Proposed Architecture

### 1. Introduce A Source Loader Boundary

Add a small abstraction between server initialization and source-specific loading:

```go
type DocumentationLoader interface {
	Load(ctx context.Context) ([]*index.Document, SourceMetadata, error)
}

type SourceMetadata struct {
	Source      string
	SourceURL   string
	Revision    string
	DocumentIDs []string
}
```

`initializeNATS` should call a NATS loader instead of directly calling `s.multiFetcher.FetchNATS(ctx)` and parsing HTML inline. This lets NATS use archive ingestion while Synadia and GitHub keep their current loaders for now.

### 2. Add A NATS Archive Loader

New package/file candidates:

- `internal/fetcher/archive.go`
- `internal/parser/gitbook.go`
- `internal/server/nats_archive_loader.go`

Responsibilities:

- Open a local archive path or download a remote archive URL.
- Validate archive safety:
  - reject absolute paths
  - reject `..` path traversal
  - cap total uncompressed bytes
  - cap file count
  - skip directories and non-source files
- Identify the archive root folder, for example `nats.docs-master/`.
- Read `.bookignore`, `SUMMARY.md`, `.gitbook.yaml`, and Markdown files.
- Produce `index.Document` values ready for `IndexNATS`.

### 3. Choose Canonical Documents From `SUMMARY.md`

Default inclusion policy:

- Include Markdown pages linked from `SUMMARY.md`.
- Include `README.md` as the root page.
- Skip ignored paths from `.bookignore`.
- Skip `zh-cn/` by default because the archive contains empty placeholder files.
- Skip `docs/`, `_examples/`, `_tools/`, and `_book/`.
- Do not include orphaned Markdown by default.

Rationale: `SUMMARY.md` is the closest source-of-truth for GitBook navigation and prevents indexing generated, hidden, duplicate, or placeholder files.

Optional config can enable broader ingestion:

- `nats_archive_include_orphans`: include non-ignored Markdown not linked from `SUMMARY.md`.
- `nats_archive_include_legacy`: include legacy redirect targets under `legacy/`.

### 4. Parse Redirects As Aliases

`.gitbook.yaml` contains redirects from old public paths to Markdown targets. The loader should parse redirects and build aliases:

```go
type DocumentAlias struct {
	AliasID      string
	DocumentID   string
	AliasURLPath string
}
```

Initial behavior:

- Add aliases for redirect targets that are already indexed.
- Allow `retrieve_nats_doc` to resolve aliases.
- Do not duplicate aliased documents in the search index.

Later behavior:

- Optionally index legacy redirect targets if `nats_archive_include_legacy` is enabled.

This preserves old doc IDs without polluting search with duplicate documents.

### 5. Map Markdown Paths To Public URLs

Use `DocsBaseURL` as the public URL base. For the archive loader, Markdown paths should map to GitBook-style URLs:

- `README.md` -> `/`
- `overview.md` -> `/overview`
- `nats-concepts/jetstream/streams.md` -> `/nats-concepts/jetstream/streams`
- `nats-concepts/jetstream/README.md` -> `/nats-concepts/jetstream`

Document IDs should match normalized public paths:

- root page ID: `index`
- `nats-concepts/jetstream/streams.md`: `nats-concepts/jetstream/streams`
- `nats-concepts/jetstream/README.md`: `nats-concepts/jetstream`

Keep the existing `normalizePath` behavior for request input.

### 6. Preprocess GitBook Markdown

Before passing Markdown to `parser.ParseMarkdown`, run a lightweight source preprocessor:

- Remove directive delimiters that add no content:
  - `{% hint style="info" %}` -> `Info:`
  - `{% hint style="warning" %}` -> `Warning:`
  - `{% endhint %}` -> empty
  - `{% tabs %}` and `{% endtabs %}` -> empty
  - `{% tab title="Go" %}` -> `### Go`
  - `{% endtab %}` -> empty
- Preserve embed URLs:
  - `{% embed url="https://..." %}` -> `Embedded: https://...`
- Preserve useful text from simple HTML fragments where possible:
  - image alt text
  - figure captions
- Leave fenced code blocks and Markdown tables intact.

This keeps indexed text useful while avoiding GitBook control syntax in snippets.

### 7. Cache Archive-Derived Documents

The existing cache can still store `[]*index.Document`, but metadata should be extended or versioned:

- source kind: `site_html` or `github_archive`
- archive URL or local archive path
- branch
- revision if known
- archive SHA256 or local file size plus mtime
- indexed document count

For remote archive downloads:

- Cache the downloaded zip separately or cache only indexed documents.
- Prefer conditional HTTP requests with `ETag` and `Last-Modified` when available.

For local archive paths:

- Treat file size, mtime, and optional SHA256 as freshness metadata.

## Configuration

Add NATS source configuration without changing the MCP tools:

```yaml
nats_source_type: archive # archive or site
nats_archive_path: assets/nats.docs-master.zip
nats_archive_url: https://github.com/nats-io/nats.docs/archive/refs/heads/master.zip
nats_archive_branch: master
nats_archive_include_orphans: false
nats_archive_include_legacy: false
```

Migration defaults:

1. First release: keep `nats_source_type: site` as default, support `archive` opt-in.
2. After parity tests: make `archive` the default.
3. Keep `site` fallback for at least one release after the default changes.

## Implementation Plan

### Phase 1: Archive Reader And Manifest Parsing

- Add archive opening and safe path validation.
- Add `.bookignore` support for the simple ignore patterns used by this repo.
- Add `SUMMARY.md` link parser:
  - parse Markdown links
  - unescape GitBook backslash escapes such as `nats\_cli`
  - preserve title and order
- Add `.gitbook.yaml` redirect parser using the existing YAML dependency.
- Add tests with a tiny zip fixture.

### Phase 2: Markdown Document Conversion

- Add GitBook preprocessor.
- Convert canonical Markdown files to `index.Document`.
- Generate doc IDs and public URLs from Markdown paths.
- Add alias map support for redirects.
- Add retrieval tests for:
  - root `README.md`
  - nested `README.md`
  - normal `.md` pages
  - redirect aliases

### Phase 3: Server Integration

- Add `DocumentationLoader` boundary.
- Route NATS initialization through either:
  - archive loader
  - existing HTML sitemap loader
- Reuse existing cache save/load and `IndexNATS`.
- Update logging to report archive revision, canonical docs, aliases, skipped files, and parse failures.

### Phase 4: Parity And Cleanup

- Compare archive-derived indexed count against current site-derived indexed count.
- Compare key public URLs:
  - `/`
  - `/overview`
  - `/nats-concepts/jetstream/streams`
  - `/running-a-nats-service/configuration/securing_nats/auth_callout`
  - `/using-nats/jetstream/nats_api_reference`
- Compare search results for representative queries:
  - `jetstream streams`
  - `auth callout`
  - `leaf nodes`
  - `nats cli`
  - `subject mapping`
- Once parity is acceptable, switch the default to archive ingestion.

## Testing Strategy

Unit tests:

- archive path safety rejects absolute and traversal paths
- archive file count and size limits
- `.bookignore` filtering
- `SUMMARY.md` link parsing and ordering
- escaped underscore path handling
- URL and document ID mapping
- redirect alias parsing
- GitBook directive preprocessing
- Markdown parser section extraction after preprocessing

Integration tests:

- load a small fixture archive and index documents
- retrieve by canonical ID
- retrieve by redirect alias
- search returns expected source and URL
- empty placeholder Markdown files are skipped

Optional local validation:

- load `assets/nats.docs-master.zip`
- assert indexed canonical docs are close to `SUMMARY.md` link count
- assert no documents are indexed from `docs/`, `_examples/`, `_tools/`, or empty `zh-cn/`

## Risks And Mitigations

- Published docs may differ from repository source.
  - Mitigation: keep the current site fetcher as fallback and run parity checks before changing defaults.
- GitBook-specific syntax may reduce section quality if not preprocessed.
  - Mitigation: add targeted preprocessor tests for hints, tabs, embeds, and HTML figures.
- Redirect aliases could create duplicate search results.
  - Mitigation: aliases should resolve retrieval only; do not duplicate index entries.
- Archive downloads may be stale or unavailable.
  - Mitigation: keep document cache, support local archive path, and support conditional HTTP cache metadata.
- Zip archives can be unsafe input.
  - Mitigation: validate paths, cap uncompressed size, cap file count, and skip unexpected file types.

## Acceptance Criteria

- With `nats_source_type: archive`, the server can initialize from `assets/nats.docs-master.zip` without network access.
- Search results for canonical NATS docs include public `docs.nats.io` URLs.
- `retrieve_nats_doc` works for root, nested README, normal Markdown, and redirect alias paths.
- Generated HTML and placeholder files are not indexed by default.
- Existing site/sitemap fetching remains available through config.
- `go test ./...` passes with the new tests.

## Open Questions

- Should legacy redirect targets be indexed by default or only available through alias resolution when already canonical?
- Should the remote archive default track `master` or should it be pinned to a release/commit for reproducibility?
- Should the server cache the raw zip, indexed documents only, or both?
- Should archive ingestion eventually replace the current generic GitHub per-file fetcher for all GitHub-backed documentation sources?
