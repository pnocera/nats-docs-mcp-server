# Review: Archive-Backed NATS Docs Implementation Plan

Date: 2026-05-11

Scope reviewed:

- `work/plans/implementation/README.md`
- `work/plans/implementation/phase-1-archive-reader.md`
- `work/plans/implementation/phase-2-markdown-conversion.md`
- `work/plans/implementation/phase-3-server-integration.md`
- `work/plans/implementation/phase-4-parity-and-cutover.md`

Overall, the plan is directionally good: archive ingestion should be simpler and more reliable than scraping the rendered GitBook site. I would not implement it exactly as written, though. The cache/source identity, alias policy, and `archive_url` config behavior need to be tightened before phase 3; otherwise archive mode can appear enabled while still serving cached site-scrape data or losing redirect behavior.

## Findings

### High: The cache fast path can bypass archive mode and drop aliases

References:

- `work/plans/implementation/phase-3-server-integration.md:132`
- `work/plans/implementation/phase-3-server-integration.md:135`
- `work/plans/implementation/phase-3-server-integration.md:140`
- `work/plans/implementation/phase-3-server-integration.md:143`
- `work/plans/implementation/phase-3-server-integration.md:163`
- `work/plans/implementation/phase-3-server-integration.md:182`
- `work/plans/implementation/phase-3-server-integration.md:184`
- `work/plans/implementation/phase-4-parity-and-cutover.md:91`
- `internal/cache/cache.go:23`
- `internal/cache/cache.go:68`

The proposed `initializeNATS` keeps a single cache key, `source := "nats"`, and returns from the cache path before calling `s.natsLoader.Load(ctx)` or `SetAliases`. The current cache schema stores `Source`, `SourceURL`, and `Documents`, but not source kind, archive origin, revision, or aliases.

This creates two failures:

1. Switching from `site` to `archive` can still load the old `nats.json` site cache and never touch `assets/nats.docs-master.zip`.
2. A fresh archive load can resolve aliases, but the next warm-cache startup loses them because aliases are not cached.

This also undermines the phase 4 parity tool: if both indices share the same cache directory and cache key, one side can accidentally measure the other side's cached documents.

Recommendation:

- Make cache identity source-specific in phase 3, not phase 4. Either namespace keys as `nats-site` and `nats-archive`, or add cache metadata (`Kind`, `Origin`, `Revision`, `Aliases`) and require it to match the selected loader before importing.
- Persist aliases with cached archive documents and call `SetAliases` on cache hit.
- Add a test where archive mode starts with an existing site cache and still loads archive docs, plus a cold/warm alias retrieval test.
- Make the parity tool use isolated temp cache dirs or explicitly disable cache for both runs.

### High: `NATSArchiveURL` is validated as usable but has no implementation path

References:

- `work/plans/implementation/phase-3-server-integration.md:15`
- `work/plans/implementation/phase-3-server-integration.md:17`
- `work/plans/implementation/phase-3-server-integration.md:63`
- `work/plans/implementation/phase-3-server-integration.md:66`
- `work/plans/implementation/phase-3-server-integration.md:84`
- `work/plans/implementation/phase-3-server-integration.md:110`
- `work/plans/implementation/phase-3-server-integration.md:111`
- `work/plans/implementation/phase-4-parity-and-cutover.md:100`

The config tests explicitly allow `archive_url`-only configuration, but the server wiring passes only `ArchivePath` into `newNATSArchiveLoader`, and phase 1/4 both defer remote archive download. That means a config can pass validation but fail at runtime because there is no URL-backed loader.

Recommendation:

- For v1, reject `NATSArchiveURL` when `NATSSourceType == "archive"` unless URL loading is actually implemented.
- Update the config tests so `archive_path` is required and `archive_url` is documented as reserved.
- If URL mode is kept, implement it fully in the loader with bounded download size and local zip caching before allowing it through validation.

### Medium: The `DocumentationIndex` alias patch conflicts with the current index shape

References:

- `work/plans/implementation/phase-2-markdown-conversion.md:198`
- `work/plans/implementation/phase-2-markdown-conversion.md:203`
- `work/plans/implementation/phase-2-markdown-conversion.md:206`
- `work/plans/implementation/phase-2-markdown-conversion.md:208`
- `internal/index/index.go:293`
- `internal/index/index.go:295`
- `internal/index/index.go:296`
- `internal/index/index.go:317`
- `internal/index/index.go:343`
- `internal/index/index.go:356`
- `internal/index/index.go:438`

The proposal's struct snippet renames `searchIndex` to `search` and redefines `mu` as alias-only protection. The current `DocumentationIndex` uses `searchIndex` throughout and also uses `di.mu` around `Index`, `Get`, `Search`, `Count`, `ExportDocuments`, and `ImportDocuments`.

If implemented literally, this either fails to compile or weakens the existing synchronization contract during a broad index change.

Recommendation:

- Keep the existing `searchIndex` field name.
- Add `aliases map[string]string` without changing the meaning of the existing `mu`, or introduce a separate `aliasMu` and be explicit about lock ordering.
- Initialize the alias map in `NewDocumentationIndex`.
- Have `SetAliases` copy the input map so callers cannot mutate the index after setting aliases.

### Medium: The legacy redirect policy contradicts the alias goal

References:

- `work/plans/implementation/README.md:46`
- `work/plans/implementation/README.md:55`
- `work/plans/implementation/README.md:100`
- `work/plans/implementation/README.md:102`
- `work/plans/implementation/phase-2-markdown-conversion.md:134`
- `work/plans/implementation/phase-2-markdown-conversion.md:138`
- `work/plans/implementation/phase-2-markdown-conversion.md:145`
- `work/plans/implementation/phase-2-markdown-conversion.md:148`
- `work/plans/implementation/phase-2-markdown-conversion.md:263`

The README says legacy redirects are not indexed by default and are "only" resolved via aliases. The phase 2 loader then drops `legacy/` documents when `includeLegacy=false` and emits aliases only when the target document is indexed.

Those two rules cannot both hold for redirects whose targets are under `legacy/`. Most of the real `.gitbook.yaml` redirects point to legacy targets, so they will be skipped by default rather than resolved via alias.

Recommendation:

- Decide the intended behavior explicitly:
  - If legacy redirects should resolve, keep legacy targets in the document store but exclude them from search results, or allow alias targets to load retrieval-only documents.
  - If legacy redirects should not resolve by default, update the README and acceptance criteria to say only redirects to indexed canonical pages resolve.
- Add tests using at least one real legacy-style redirect and one canonical redirect.

### Medium: The GitBook preprocessor fence instruction is likely reversed

References:

- `work/plans/implementation/phase-2-markdown-conversion.md:43`
- `work/plans/implementation/phase-2-markdown-conversion.md:244`

The plan says to split on triple-backtick fences and run regex passes on odd-indexed "non-code" chunks. With typical split/token approaches, chunk parity is easy to get wrong, and the first chunk before any fence is outside code. Implementing this as written risks stripping `{% ... %}` examples inside code blocks while leaving real directives untouched.

Recommendation:

- Use a line-oriented state machine that toggles on fenced code open/close lines, rather than relying on split-index parity.
- Preserve fenced blocks byte-for-byte and run all directive rewrites only while outside a fence.
- Include tests with directives before, inside, and after a fenced block. Supporting `~~~` fences is cheap and worth adding.

### Low: The zip traversal rule is imprecise

References:

- `work/plans/implementation/phase-1-archive-reader.md:67`
- `work/plans/implementation/phase-1-archive-reader.md:68`
- `work/plans/implementation/phase-1-archive-reader.md:74`
- `work/plans/implementation/phase-2-markdown-conversion.md:174`

`strings.Contains(path.Clean(name), "..")` is not the right model for "escapes the root." It rejects valid path segments such as `notes..old.md`, and it couples root validation to substring matching rather than path structure.

Recommendation:

- Normalize slashes, reject absolute paths and NUL bytes, clean the path, split into segments, and reject any segment equal to `..`.
- Determine the expected top-level root directory, require every regular file to be under `root/`, then store only the relative path below that root.
- Keep the traversal tests, but add a non-traversal filename containing `..` so the safety check does not become overbroad.

## Additional Notes

- `README.md:32` mentions a checked-in `internal/server/testdata/nats-fixture.zip`, while `README.md:85` and `phase-2-markdown-conversion.md:319` say the fixture is built programmatically and no binary fixture is checked in. Pick one; programmatic fixture generation is the cleaner choice.
- The plan should update config/property tests beyond the hand-written table tests, because this repo already has extensive config property coverage.
- Phase 4 should compare cache-disabled or cache-isolated indices. Otherwise the parity report can look good while both sides are reading the same cached `nats` document set.

## Suggested Plan Changes Before Implementation

1. Fix phase 3 cache behavior first: source identity, aliases, and cache-hit validation.
2. Make URL mode either unsupported in validation or fully implemented.
3. Revise the `DocumentationIndex` patch to preserve the current struct fields and locking pattern.
4. Clarify whether legacy redirects should resolve by default.
5. Replace the preprocessor fence guidance with an explicit state-machine approach.
