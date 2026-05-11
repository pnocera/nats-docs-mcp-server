# Phase 4 — Parity Testing and Default Cutover

**Outcome:** evidence that archive ingestion produces equivalent or better coverage than the live HTML scrape, then a default-flag flip and a cleanup pass. The legacy `site` loader stays available for one release after the flip.

Depends on phase 3.

## Parity tooling

### New: `scripts/parity_check.go` (or a `cmd/parityctl` subcommand)

A small standalone Go program that:

1. Boots two indices side by side using the same `internal/server` types, each with an **isolated temp cache directory** (or `--no-cache` flag set in the tool to bypass cache entirely). This is mandatory — sharing a cache dir between the two boots would let one side measure the other side's documents through the cache layer, even with source-namespaced keys, if file ordering is unlucky during refresh. Two separate `t.TempDir()`-style directories cost nothing and remove the doubt.
   - **A:** `nats_source_type=site`, isolated cache, `MaxConcurrent` low (3) and a 60s `FetchTimeout`.
   - **B:** `nats_source_type=archive`, `nats_archive_path=assets/nats.docs-master.zip`, isolated cache.
2. Records, for each index:
   - total doc count
   - set of document IDs
   - for a hand-picked set of canonical pages, the URL and the first ~500 chars of joined section content
3. Prints a report:
   ```
   site:    187 documents
   archive: 191 documents
   site-only IDs:    [...]    (5)
   archive-only IDs: [...]    (9)
   common IDs:       182
   ```
4. For each canonical page, prints both URLs and a unified-style diff of the content snippet so a human can eyeball whether the archive output is sensible.

This is a developer tool, not a server feature. It does not ship in releases; living under `scripts/` makes that clear. Single file, no tests of its own; it exercises the production code paths.

### Canonical pages to spot-check

From the proposal:

- `/`
- `/overview`
- `/nats-concepts/jetstream/streams`
- `/running-a-nats-service/configuration/securing_nats/auth_callout`
- `/using-nats/jetstream/nats_api_reference`

Add three more high-traffic candidates:

- `/nats-concepts/subjects`
- `/using-nats/developing-with-nats/connecting/connect`
- `/release_notes/whats_new`

For each, assert:

1. The document exists in both indices.
2. URLs match exactly.
3. Content lengths are within 30% of each other (the archive may include slightly more or less prose; gross divergence indicates a parser bug).

### Search query parity

For these queries, run `s.orchestrator.Search(query, 10)` against both indices and require:

- Top-3 result URLs from the archive index include at least 2 of the top-3 from the site index.

Queries (from the proposal):

- `jetstream streams`
- `auth callout`
- `leaf nodes`
- `nats cli`
- `subject mapping`

This is a soft check — rankings can shift slightly. Use it to spot pathological regressions, not to block on minor reordering.

## Cutover steps

1. **Add `assets/nats.docs-master.zip` to the repo.** It's already there in the working tree but confirm it's tracked or fetch-on-build wired up. If it's not committed, decide between checking in (5.5 MB — workable) and adding a `make fetch-archive` target that downloads a pinned commit. **Recommendation:** check it in for v1 so offline tests stay hermetic. Update `.gitattributes` if LFS is used; otherwise plain commit is fine.
2. **Run the parity tool.** Iterate on the preprocessor or filters until the report looks sensible.
3. **Flip the default in `internal/config/config.go`:**
   ```go
   NATSSourceType:  "archive",
   NATSArchivePath: "assets/nats.docs-master.zip",
   ```
4. **Update `config.example.yaml`** to reflect the new default. Add a comment block showing how to revert to `site`.
5. **Update `README.md`** with a short subsection: "NATS documentation source — archive vs site." Three paragraphs maximum.
6. **CI smoke test:** add one CI step that boots the server with `archive` mode, calls `search_nats_docs` for "jetstream", and asserts a non-empty result. Use the existing test-binary plumbing (`go test ./cmd/...` if present, otherwise a `go run ./cmd/nats-docs-mcp-server` driven by a short script).

## Cleanup

After the cutover lands and one release has shipped:

- **Keep** `natsSiteLoader` and the `multiFetcher.FetchNATS` path. They remain the fallback for at least one more release per the proposal's migration policy.
- **Remove** any test-only `TODO: switch default` markers added during phases 2–3.
- **Delete** the parity tool only if it's clearly orphaned. Leaving it in `scripts/` costs nothing and helps debug future archive drift.

## Optional: remote archive download

Wiring `NATSArchiveURL` end-to-end requires:

- HTTP GET with `If-None-Match` / `If-Modified-Since` against the previous response stored next to the cache.
- Reusing `ArchiveLimits` to bound the response body.
- A local cache file under `cfg.GetCacheDir() + "/archive/nats.zip"`.
- Falling back to the cached zip if the network is unreachable.
- Removing the phase-3 validation rule that rejects a non-empty `NATSArchiveURL`.

Defer unless someone asks for it. For dev workflows, `nats_archive_path` is enough. Until then, `Validate()` continues to reject `NATSArchiveURL` — better than a config field that silently does nothing.

## Risks revisited

- **Published docs lag the repo.** Mitigation: the user can pin `nats_archive_branch` to a release tag, or revert to `nats_source_type=site` per-config. The migration kept both paths available specifically for this.
- **Archive download flake.** Mitigation: in v1 we don't download — local path only. Phase 4 doesn't change that.
- **Search-result drift.** Mitigation: the parity check exposes it pre-cutover. If drift looks bad, hold the cutover and iterate on the preprocessor.
- **Empty or hostile zip.** Mitigation: `ArchiveLimits` enforced in phase 1; zero-document load fails fast in `initializeNATS`.

## Acceptance criteria for phase 4 (and the project)

- Parity report shows ≥95% canonical doc-ID overlap between `site` and `archive`. Any deltas are explained (intentional skip, generated page, etc).
- Eight spot-checked canonical pages return non-empty, non-corrupted content under `archive` mode.
- Five sample search queries return reasonable top-3 results under `archive` mode.
- Default config boots the server using the local archive with no network access; `search_nats_docs` and `retrieve_nats_doc` both work.
- All existing `go test ./...` passes.
- `README.md` and `config.example.yaml` document the new default and the fallback path.

## Out of scope

- New ranking algorithm.
- Synadia/GitHub archive ingestion.
- Web UI changes (none exist).
- Telemetry beyond the existing zerolog lines.
