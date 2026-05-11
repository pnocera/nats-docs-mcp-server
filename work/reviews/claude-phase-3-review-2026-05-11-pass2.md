# Phase 3 Review — Pass 2 (post-fix)

`go build ./...` clean; `go test -count=1 ./...` green across cache, config, server, and the rest.

## Blocking / High severity
*None.*

## Medium severity
*None.*

## Low severity (non-blocking)

1. **Cache fast path still reads the file twice** — `internal/server/server.go:254,262`
   Pass-1 finding #1 not addressed. `cache.IsValid(source, maxAge)` already calls `Load()` internally, then the warm path calls `s.cache.Load(source)` again. Cosmetic perf, not correctness.

2. **Dead branch on empty-documents cache** — `internal/server/server.go:266-267`
   `IsValid` returns `(false, nil)` when `len(cached.Documents) == 0` (`internal/cache/cache.go:203-205`), so the outer `else if valid` guard means `case len(cached.Documents) == 0:` can never fire. Harmless defensive arm; safe to keep or drop.

3. **Validation error message style mismatch** — `internal/config/config.go:641` vs `:644`
   Line 641 uses YAML-path form (`nats.archive_path`, `nats.source_type`); line 644 uses env/flag form (`nats_archive_url`, `nats_source_type`). Same field, two spellings in adjacent strings. Trivial.

4. **`.gitignore` still missing trailing newline** — `.gitignore:44`. Re-flagged from pass 1, unchanged.

5. **Property-test generators still don't fuzz the new NATS fields** — `internal/config/config_property_test.go`
   Pass-1 finding #3 not addressed. Tests still pass because `NewConfig()` defaults to a valid source, but the new fields aren't fuzzed. Coverage polish; the unit tests in `config_test.go:768-849` cover the validation matrix explicitly.

## Pass-1 follow-ups confirmed fixed

- **Tool-surface alias coverage** — `TestRetrieveToolHandlerWithAlias` (`internal/server/tools_test.go:296-340`) now drives the alias through `handleRetrieveTool` end-to-end (seeds doc, calls `SetAliases`, asserts `TextContent` contains the canonical title). Plugs pass-1 finding #4.

## Re-spot-checked (still correct)

- **Cache key isolation** holds: `natsCacheSourceForType`/`otherNATSCacheSource` (`server.go:331-343`); `TestInitialize_ModeSwitch_SiteToArchive`/`...ArchiveToSite` (`initialize_archive_test.go:132-190`) and `TestInitialize_RefreshCacheClearsOtherMode` (`:204-219`) lock in the contract.
- **Kind guard** at `server.go:268` accepts empty Kind (v1.0 caches) while rejecting cross-mode Kind. v1.0 round-trip is covered by `TestCacheLoadV10Compatibility` (`cache_test.go:191-220`); cross-mode reject by `TestInitialize_ArchiveSource_KindMismatchInvalidatesCache` (`initialize_archive_test.go:116-130`).
- **Alias persistence**: warm path `SetAliases(cached.Aliases)` (`server.go:275`); cold path `SetAliases(meta.Aliases)` (`server.go:302`). `SetAliases` always allocates a fresh map (`index/index.go:360-369`), so `nil` from v1.0 caches safely clears any prior aliases — no stale bleed across mode switches.
- **`copyAliases`** at `cache.go:281-290` snapshots before serialization; `TestCacheSaveWithMetadataRoundTrip` (`cache_test.go:156-188`) mutates the input map post-Save and confirms the cached value is unaffected.
- **`SaveWithMetadata`/`Save` back-compat**: legacy `Save(source, url, docs)` now delegates to `SaveWithMetadata` with empty Kind/Origin/Revision/Aliases (`cache.go:82-87`); Synadia/GitHub call sites are unchanged and continue writing v1.1 with empty Kind, which the kind-guard treats as accept.
- **`validateCachedDocuments`** accepts `"1.0"` or current `cacheVersion` (`cache.go:259`); future bumps still fail closed.
- **Validation matrix**: invalid type, archive+empty path, archive+url-reserved, archive path-only, site ignores archive URL — all covered (`config_test.go:768-849`).
- **Env/file/flag plumbing** for the six new fields is symmetric across `loadFromEnv` (`config.go:445-462`), `loadNATSArchiveFromViper` (`config.go:741-759`), and `LoadWithFlags` (`config.go:327-356`), with `parseBoolEnv` accepting `true/1/yes/y/on`.
- **`RefreshCache` cross-mode clear** (`server.go:245-249`) only runs under refresh, leaving the other mode's cache intact on normal init — covered by mode-switch tests.
- **Offline archive path** still verified by `TestInitialize_ArchiveSource_OfflineNoNetwork` (`initialize_archive_test.go:192-202`).

## Verdict

**GO** — pass-2 fixes the one actionable gap from pass 1 (tool-surface alias coverage). Remaining items are cosmetic (double-Load, dead switch arm, error-string casing, gitignore newline) or coverage polish (property-test generators). None block merging phase 3.
