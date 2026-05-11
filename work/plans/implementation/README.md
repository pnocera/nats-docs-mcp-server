# Implementation Plan: Archive-Backed NATS Docs Ingestion

Date: 2026-05-11
Source proposal: `../nats-docs-archive-ingestion-proposal-2026-05-11.md`
Archive analysed: `assets/nats.docs-master.zip` (5.5 MB, 287 Markdown files, 261 non-empty).

## Goal in one sentence

Replace the per-page HTML scraper for `docs.nats.io` with a single archive read of `nats-io/nats.docs`, parsed as Markdown through the existing Goldmark pipeline, exposed behind a config flag so the legacy site loader stays available until parity is proven.

## Phasing

| Phase | File                                | Outcome                                                                  |
|-------|-------------------------------------|--------------------------------------------------------------------------|
| 1     | `phase-1-archive-reader.md`         | Safe zip reader + parsers for `SUMMARY.md`, `.bookignore`, `.gitbook.yaml`. |
| 2     | `phase-2-markdown-conversion.md`    | GitBook preprocessor + Markdown-to-`index.Document` conversion + aliases. |
| 3     | `phase-3-server-integration.md`     | `DocumentationLoader` boundary, NATS loader wiring, cache + logging.     |
| 4     | `phase-4-parity-and-cutover.md`     | Parity checks against the site loader, default switch, cleanup.          |

Each phase is independently reviewable and shippable. Phase 1 + 2 build a self-contained package with tests against a fixture zip. Phase 3 wires it in behind `nats_source_type=archive` (opt-in). Phase 4 flips the default and removes scaffolding.

## Package layout (new)

- `internal/fetcher/archive.go` — zip opening, safe extraction, archive metadata.
- `internal/parser/gitbook.go` — GitBook directive preprocessor (pure string -> string).
- `internal/parser/summary.go` — `SUMMARY.md` link extraction.
- `internal/parser/bookignore.go` — ignore-pattern matcher.
- `internal/parser/gitbook_redirects.go` — `.gitbook.yaml` redirect parser.
- `internal/server/loader.go` — `DocumentationLoader` interface + shared types.
- `internal/server/nats_site_loader.go` — current HTML path moved behind the loader interface.
- `internal/server/nats_archive_loader.go` — new archive loader.
- (fixture archive is built programmatically inside tests — no binary checked in; see "Test scaffolding" below).

Files modified, not created:

- `internal/config/config.go` — add NATS source fields, env vars, validation.
- `internal/server/server.go` — replace inline `initializeNATS` body with loader call.
- `internal/cache/cache.go` — extend `CachedDocuments` metadata (additive, backwards-compatible).
- `internal/index/index.go` — add alias resolution to `DocumentationIndex.Get` (additive).
- `config.example.yaml` — document new keys.

## Key design decisions (locking these in upfront)

1. **Loader boundary lives in `internal/server`, not a new top-level package.** It's a thin seam between `Initialize*` and source-specific code; pulling it out further would force config types to leak. The interface is small (`Load(ctx) ([]*index.Document, SourceMetadata, error)`).
2. **Archive ingestion only applies to the NATS source for now.** Synadia and GitHub keep their existing fetchers. The proposal's question about replacing the GitHub fetcher is deferred — same archive plumbing can be reused later but is out of scope.
3. **Aliases live on the index, not as duplicate documents, and only resolve when the target is canonical.** Add a `map[string]string` alias-to-canonical lookup checked in `DocumentationIndex.Get`. A redirect entry is emitted only when its target is in the indexed document set. **Consequence:** most `.gitbook.yaml` redirects point under `legacy/`; with the default `nats_archive_include_legacy=false` those targets are not indexed and the aliases are silently dropped. To get legacy redirects to resolve, the operator must set `nats_archive_include_legacy=true`. This is documented in the acceptance criteria and surfaced in the loader's log line (`aliases_dropped_legacy=N`).
4. **GitBook preprocessing is text-level, run before `parser.ParseMarkdown`.** Goldmark sees only valid Markdown. This avoids adding a Goldmark extension and keeps the change surface in one file.
5. **Default source stays `site` for the first release.** Switch to `archive` in phase 4 once parity tests pass. The `site` fallback is kept for at least one release after the flip — per the proposal's migration table.
6. **Local archive path is the v1 ingress; `nats_archive_url` is reserved and rejected by validation.** Remote download is not implemented in v1. The config field is documented as reserved for a future change. `Validate()` requires `nats_archive_path` to be set when `nats_source_type=archive` and rejects `nats_archive_url` if it is non-empty. This prevents the trap where a URL-only config passes validation but the loader has nothing to load.
7. **Cache identity is source-specific from day one.** The cache key for NATS is `nats-archive` or `nats-site`, not bare `nats`. The cached payload also records `Kind` and `Origin`; on cache hit, the loader verifies the cached `Kind` matches the configured source type and otherwise treats it as a miss. Aliases are persisted alongside documents (`CachedDocuments.Aliases`) and reinstalled via `SetAliases` after cache import. This is required in phase 3, not deferred to phase 4 — without it, switching modes silently serves stale data from the other mode.

## Open questions deferred from the proposal

- **Pin or track `master`?** Default to `master` for parity with the current published site; allow `nats_archive_branch` override. Pinning to a SHA is a config decision, not a code change.
- **Cache the raw zip?** No, only cache the indexed `[]*index.Document` plus the alias map. The zip is small enough to refetch and caching both doubles the freshness-checking surface.
- **Index legacy redirects by default?** No, and **redirects to legacy targets do not resolve under default config**. The user must opt in via `nats_archive_include_legacy=true`, which both indexes the legacy pages and lets their aliases resolve. This is a behaviour change from the proposal text ("only available through alias resolution"), made because aliases that point at non-indexed documents cannot resolve through the index — a key invariant of the alias model is that the target must exist.

## Out of scope

- Search ranking changes.
- MCP tool interface changes.
- Synadia archive ingestion.
- Replacing the generic GitHub fetcher.
- Indexing `_examples/` or `docs/` generated output.

## Test scaffolding

A single synthetic fixture archive powers both unit and integration tests. It is built programmatically by test helpers and never checked in as a binary file. Contents:

```
nats-fixture/
  .bookignore                    # _book/, docs/, _examples/, building_the_book.md
  .gitbook.yaml                  # one redirect: legacy/old -> legacy/old.md
  SUMMARY.md                     # links to README.md, overview.md, nested/a.md, nested/b/README.md
  README.md                      # root page, "# Welcome"
  overview.md                    # uses {% hint style="info" %} ... {% endhint %}
  nested/a.md                    # uses {% tabs %}{% tab title="Go" %}...{% endtab %}{% endtabs %}
  nested/b/README.md             # nested README mapping
  legacy/old.md                  # redirect target
  zh-cn/placeholder.md           # empty file, must be skipped
  docs/generated.html            # must be skipped
  _examples/sample.html          # must be skipped
  orphan.md                      # not linked from SUMMARY.md, must be skipped by default
```

A helper in `internal/server/testdata/fixture.go` (or a `TestMain` in the loader test) builds the zip programmatically so we don't check in a binary. The total size stays under 5 KB.

## Sequencing & estimates

- **Phase 1:** ~1 day. Self-contained; lots of unit tests.
- **Phase 2:** ~1 day. Mostly preprocessor regex work plus the document-conversion happy path.
- **Phase 3:** ~0.5 day. Wiring + config + logging. No new behaviour, just plumbing.
- **Phase 4:** ~0.5 day plus eyeball time. Mostly running the server with both flags and diffing.

Total: roughly 3 working days plus parity validation.

## Acceptance criteria (full project)

Lifted verbatim from the proposal:

- With `nats_source_type: archive`, the server initializes from `assets/nats.docs-master.zip` without network access.
- Search results for canonical NATS docs include public `docs.nats.io` URLs.
- `retrieve_nats_doc` works for root, nested README, normal Markdown, and canonical redirect alias paths; legacy redirect aliases require `nats_archive_include_legacy=true`.
- Generated HTML and placeholder files are not indexed by default.
- Existing site/sitemap fetching remains available through config.
- `go test ./...` passes with the new tests.
