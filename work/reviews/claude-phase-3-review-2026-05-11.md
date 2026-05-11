Builds clean and `go test -count=1 ./...` is green (server, cache, config, fetcher, parser, etc.).

# Phase 3 Review — findings

## Blocking / High severity
*None.*

## Medium severity
*None.*

## Low severity (non-blocking, worth knowing)

1. **Cache fast path reads the file twice** — `internal/server/server.go:254-261`
   `cache.IsValid(source, maxAge)` already calls `Load()` internally to compute validity, then the server calls `s.cache.Load(source)` again. Cosmetic perf hit; not a correctness issue.

2. **`nats.json` (old cache key) is orphaned after upgrade** — `internal/server/server.go:242`
   Pre-phase-3 NATS cache lived under key `"nats"` (file `nats.json`). After this change the keys are `nats-site`/`nats-archive`, so existing users do a one-time re-fetch and `nats.json` is left on disk. The plan implicitly accepts this (mode-isolation goal), and v1.0 schema is still loadable per `validateCachedDocuments` (cache.go:259) — but the old file is never reached because the key changed. Worth a release-note mention; no code fix required.

3. **Property-test generators not extended for the new NATS fields** — `internal/config/config_property_test.go`
   Plan said: *"extend the relevant generators so randomly generated configs include the new fields and the property tests still pass."* The file is unchanged. Tests still pass because `NewConfig()` defaults to a valid `NATSSourceType="site"`, but the new fields aren't fuzzed.

4. **`tools_test.go` alias coverage gap** — `internal/server/tools_test.go`
   Plan asked for a parallel `handleRetrieveTool` case that hits a redirect alias. Not added. Alias resolution is still covered at the index layer in `initialize_archive_test.go:82-94, 96-114` via `GetNATSIndex().Get("overview-old")`, so behavior is verified — just not through the MCP tool surface.

5. **Kind-mismatch test doesn't cover the v1.0 sub-case** — `internal/server/initialize_archive_test.go:116-130`
   Plan suggested seeding either a v1.1 kind mismatch *or* a v1.0 cache from the site path. Only the v1.1 case is exercised here. V1.0 round-trip is covered separately by `TestCacheLoadV10Compatibility` in `internal/cache/cache_test.go:191-220`, so this is acceptable.

6. **`.gitignore` missing trailing newline** — `.gitignore:44`. Trivial.

## Things that look right (spot-checked, no issue)

- **Loader selection**: `internal/server/server.go:147-160` correctly switches on `cfg.NATSSourceType` and falls through to `natsSiteLoader` by default; `TestServer_NATSArchiveSelected` / `TestServer_NATSSiteSelectedByDefault` lock this in.
- **Cache source isolation**: `natsCacheSourceForType` + `otherNATSCacheSource` namespacing (server.go:340-353); mode-switch tests confirm no cross-mode hit (initialize_archive_test.go:132-190).
- **Kind guard**: `cached.Kind != "" && cached.Kind != expectedKind` (server.go:266-268) treats unknown (v1.0) as accept-and-continue while rejecting cross-mode kind matches — matches the plan exactly.
- **Alias persistence**: warm path calls `SetAliases(cached.Aliases)` (server.go:275); cold path calls `SetAliases(meta.Aliases)` after indexing (server.go:299). `SetAliases(nil)` safely resets, so no stale alias bleed.
- **`Cache.SaveWithMetadata` + back-compat `Save` wrapper** (cache.go:82-87, 90-146): Synadia/GitHub paths untouched and continue to write `Kind=""`, which is a no-op for those source keys.
- **`copyAliases`** (cache.go:281-290) defensively snapshots before serialization — `TestCacheSaveWithMetadataRoundTrip` proves the input map can mutate after Save without corrupting the cache.
- **Validation rules** match the plan: invalid type rejected, archive+empty-path rejected, archive+url rejected with "reserved" wording, site mode ignores archive_url (config.go:637-649; tests in config_test.go:782-832).
- **Env / file / flag plumbing** for the six new fields is symmetric: `loadFromEnv` (config.go:445-462), `loadNATSArchiveFromViper` (config.go:741-759), `LoadWithFlags` (config.go:327-356). `parseBoolEnv` accepts `true/1/yes/y/on` (config.go:761-768).
- **RefreshCache clears the other mode** (server.go:245-249) — covered by `TestInitialize_RefreshCacheClearsOtherMode`.
- **Offline archive path** test (initialize_archive_test.go:192-202) confirms no network is required when `DocsBaseURL` is unreachable.
- **`config.example.yaml`** documents the `archive_url is reserved` policy inline (config.example.yaml:15-17).

## Verdict

**GO** — phase 3 meets the plan's correctness, isolation, validation, and back-compat goals; the low-severity items above are documentation / coverage polish and can be folded into phase 4 without blocking.
