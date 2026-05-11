# Phase 3 — Server Integration

**Outcome:** the server picks between `natsArchiveLoader` and `natsSiteLoader` based on config. New behaviour is fully opt-in via `nats_source_type=archive`. Default remains `site`. Cache, retrieve, and search all keep working unchanged.

Depends on phases 1 and 2.

## Config schema additions

### Modified: `internal/config/config.go`

Add fields to `Config`:

```go
// NATS source settings
NATSSourceType            string // "site" or "archive" (default "site")
NATSArchivePath           string // local path to nats.docs zip; required when source=archive
NATSArchiveURL            string // RESERVED for future remote download; rejected in v1
NATSArchiveBranch         string // documented for future remote use (default "master")
NATSArchiveIncludeOrphans bool   // include non-SUMMARY-linked .md files (default false)
NATSArchiveIncludeLegacy  bool   // include legacy/ targets as canonical docs (default false)
```

Update `NewConfig()` to set defaults:

```go
NATSSourceType:            "site",
NATSArchivePath:           "",
NATSArchiveURL:            "",
NATSArchiveBranch:         "master",
NATSArchiveIncludeOrphans: false,
NATSArchiveIncludeLegacy:  false,
```

Update `LoadFromFile` to read these keys:

```yaml
nats:
  source_type: archive
  archive_path: assets/nats.docs-master.zip
  archive_url: ""
  archive_branch: master
  archive_include_orphans: false
  archive_include_legacy: false
```

Add the same reads inside `LoadWithFlags`'s config-file branch.

Add to `loadFromEnv` (mirroring the existing pattern):

```go
if val := getEnv("NATS_SOURCE_TYPE"); val != "" { cfg.NATSSourceType = val }
if val := getEnv("NATS_ARCHIVE_PATH"); val != "" { cfg.NATSArchivePath = val }
if val := getEnv("NATS_ARCHIVE_URL"); val != "" { cfg.NATSArchiveURL = val }
if val := getEnv("NATS_ARCHIVE_BRANCH"); val != "" { cfg.NATSArchiveBranch = val }
if val := getEnv("NATS_ARCHIVE_INCLUDE_ORPHANS"); val != "" {
    cfg.NATSArchiveIncludeOrphans = val == "true" || val == "1" || val == "yes"
}
if val := getEnv("NATS_ARCHIVE_INCLUDE_LEGACY"); val != "" {
    cfg.NATSArchiveIncludeLegacy = val == "true" || val == "1" || val == "yes"
}
```

Add `Validate()` rules:

- `NATSSourceType` must be `"site"` or `"archive"`.
- If `NATSSourceType == "archive"`:
  - `NATSArchivePath` must be non-empty (path is the only supported ingress in v1).
  - `NATSArchiveURL` must be empty — it is reserved for a future change. Validation produces an error like `nats_archive_url is reserved and not supported in this release; remove the field or set nats_source_type=site`. This is the explicit guard the review called out: better to fail at config-load than at runtime with a confusing message.
- If `NATSArchivePath` is set and the file does not exist at startup time, that's a runtime error from the loader, not a config-validation error (keeps `Validate()` pure and friendly to test setups that build the file later).

### Modified: `config.example.yaml`

Add a commented-out `nats:` block illustrating the new keys. Keep the file's existing structure intact.

### Tests: `internal/config/config_test.go` (and existing test files)

Cover:

| Test                                          | Asserts                                                              |
|-----------------------------------------------|----------------------------------------------------------------------|
| `TestNewConfig_NATSSourceDefaults`            | Defaults: `site`, empty path/url, branch `master`, both flags false. |
| `TestLoadFromFile_NATSArchiveSection`         | Parsing a YAML with the new `nats:` block populates the fields.      |
| `TestLoadFromEnv_NATSArchiveEnvVars`          | `NATS_DOCS_NATS_SOURCE_TYPE=archive` etc. populate fields.           |
| `TestValidate_InvalidSourceType`              | `NATSSourceType="foo"` -> validation error.                          |
| `TestValidate_ArchiveSourceMissingPath`       | `archive` with empty `archive_path` -> validation error.             |
| `TestValidate_ArchiveSourceRejectsURL`        | `archive` with `archive_url` non-empty -> validation error citing "reserved". |
| `TestValidate_ArchiveSourcePathOnly`          | `archive` with only `archive_path` -> no error.                      |
| `TestValidate_SiteSourceIgnoresArchiveFields` | `site` with `archive_url` set -> no error (only enforced under `archive`). |

Match the existing test style (table-driven where the file already uses tables). The repo already has property-style coverage in `config_property_test.go`; extend the relevant generators so randomly generated configs include the new fields and the property tests still pass.

## Server wiring

### Modified: `internal/server/server.go`

#### Build the loader once in `NewServer`

Add a private field:

```go
type Server struct {
    ...
    natsLoader DocumentationLoader
}
```

Inside `NewServer`, after the `multiFetcher` is constructed, choose the loader:

```go
var natsLoader DocumentationLoader
switch cfg.NATSSourceType {
case "archive":
    natsLoader = newNATSArchiveLoader(natsArchiveConfig{
        ArchivePath:    cfg.NATSArchivePath,
        DocsBaseURL:    cfg.DocsBaseURL,
        IncludeOrphans: cfg.NATSArchiveIncludeOrphans,
        IncludeLegacy:  cfg.NATSArchiveIncludeLegacy,
    }, logger)
default:
    natsLoader = &natsSiteLoader{
        fetcher:     multiFetcher,
        docsBaseURL: cfg.DocsBaseURL,
        logger:      logger,
    }
}
```

Store on the struct and pass to `initializeNATS`.

#### Rewrite `initializeNATS`

Replace lines 220–300. The new shell uses source-namespaced cache keys, validates the cached payload's `Kind` before importing, and persists/restores aliases. `source := "nats-" + cfg.NATSSourceType` ensures site and archive modes never share a cache file.

```go
func (s *Server) initializeNATS(ctx context.Context) error {
    source := "nats-" + s.config.NATSSourceType // "nats-site" or "nats-archive"
    expectedKind := kindForSourceType(s.config.NATSSourceType) // "site_html" or "github_archive"

    // Cache fast path — requires the cached payload's Kind to match the
    // currently configured source type. Otherwise treat it as a miss so that
    // switching modes never silently serves stale data from the other mode.
    if !s.config.RefreshCache && s.cache != nil {
        maxAge := time.Duration(s.config.CacheMaxAge) * 24 * time.Hour
        if valid, _ := s.cache.IsValid(source, maxAge); valid {
            cached, err := s.cache.Load(source)
            switch {
            case err != nil:
                s.logger.Warn("Cache load failed, will reload", "source", source, "error", err)
            case len(cached.Documents) == 0:
                s.logger.Info("Empty cache, will reload", "source", source)
            case cached.Kind != "" && cached.Kind != expectedKind:
                s.logger.Info("Cache kind mismatch, will reload",
                    "source", source, "cached_kind", cached.Kind, "expected_kind", expectedKind)
            default:
                if err := s.indexManager.GetNATSIndex().ImportDocuments(cached.Documents); err != nil {
                    s.logger.Warn("Failed to import cached docs, will reload", "error", err)
                } else {
                    s.indexManager.GetNATSIndex().SetAliases(cached.Aliases)
                    s.logger.Info("Loaded NATS docs from cache",
                        "source", source,
                        "count", len(cached.Documents),
                        "aliases", len(cached.Aliases),
                        "cached_at", cached.CachedAt)
                    return nil
                }
            }
        }
    }

    s.logger.Info("Loading NATS documentation",
        "source_type", s.config.NATSSourceType)

    docs, meta, err := s.natsLoader.Load(ctx)
    if err != nil {
        return fmt.Errorf("failed to load NATS documentation: %w", err)
    }
    if len(docs) == 0 {
        return fmt.Errorf("NATS loader returned zero documents")
    }

    if err := s.indexManager.IndexNATS(docs); err != nil {
        return fmt.Errorf("failed to index NATS documentation: %w", err)
    }
    s.indexManager.GetNATSIndex().SetAliases(meta.Aliases)

    s.logger.Info("NATS documentation indexed",
        "count", len(docs),
        "kind", meta.Kind,
        "origin", meta.Origin,
        "revision", meta.Revision,
        "aliases", len(meta.Aliases),
        "aliases_dropped", meta.AliasesDropped)

    if s.cache != nil {
        if err := s.cache.SaveWithMetadata(source, cache.SaveParams{
            SourceURL: meta.SourceURL,
            Kind:      meta.Kind,
            Origin:    meta.Origin,
            Revision:  meta.Revision,
            Documents: docs,
            Aliases:   meta.Aliases,
        }); err != nil {
            s.logger.Warn("Failed to save cache", "source", source, "error", err)
        }
    }

    return nil
}

func kindForSourceType(s string) string {
    if s == "archive" {
        return "github_archive"
    }
    return "site_html"
}
```

#### Cache schema additions (required, not deferred)

`internal/cache/cache.go` must persist aliases and source kind so cache hits can re-arm `SetAliases` and detect mode mismatches:

```go
const cacheVersion = "1.1" // bump from 1.0

type CachedDocuments struct {
    Version       string            `json:"version"`
    Source        string            `json:"source"`         // "nats-site" or "nats-archive"
    SourceURL     string            `json:"source_url"`
    Kind          string            `json:"kind,omitempty"` // new in 1.1
    Origin        string            `json:"origin,omitempty"`
    Revision      string            `json:"revision,omitempty"`
    CachedAt      time.Time         `json:"cached_at"`
    DocumentCount int               `json:"document_count"`
    Documents     []*index.Document `json:"documents"`
    Aliases       map[string]string `json:"aliases,omitempty"` // new in 1.1
}

// SaveParams bundles the extra metadata.
type SaveParams struct {
    SourceURL string
    Kind      string
    Origin    string
    Revision  string
    Documents []*index.Document
    Aliases   map[string]string
}

// SaveWithMetadata is the v1.1 entry point. Save() is kept as a thin wrapper
// that calls SaveWithMetadata with empty metadata, so the Synadia and GitHub
// init paths don't need to change.
func (c *Cache) SaveWithMetadata(source string, p SaveParams) error
```

Backwards compatibility: cache files from v1.0 unmarshal cleanly because the new fields are `omitempty`/optional. A `1.0` payload loaded with the new code has `Kind=""`, which the cache-hit check treats as "kind unknown — accept and continue" (the `cached.Kind != ""` guard above). Once the entry is rewritten with new metadata it becomes a v1.1 entry.

`Synadia` and `GitHub` init paths keep calling `c.Save(source, sourceURL, docs)`. Implement `Save` as `c.SaveWithMetadata(source, SaveParams{SourceURL: sourceURL, Documents: docs})`. No behavioural change for non-NATS sources.

#### Stale cache cleanup at startup (low cost, high payoff)

When `RefreshCache` is true, also clear the *other* mode's cache file. Otherwise a user that flips `nats_source_type` and runs with `--refresh-cache` will leave a stale `nats-site.json` lying around forever. One `os.Remove` call on the unused key; ignore not-found errors.

#### Synadia / GitHub paths

Leave `initializeSynadia` and `initializeGitHub` untouched. They retain the existing fetcher-based code. The loader interface is only adopted by NATS in this change.

### RefreshCache

`RefreshCache` calls `initializeNATS`, so it picks up the new loader automatically. No code change needed beyond verifying it still compiles.

## Tests

### `internal/server/server_test.go` additions

| Test                                  | Asserts                                                                  |
|---------------------------------------|--------------------------------------------------------------------------|
| `TestServer_NATSArchiveSelected`      | `cfg.NATSSourceType="archive"` -> `s.natsLoader` is `*natsArchiveLoader`. |
| `TestServer_NATSSiteSelectedByDefault`| Default config -> `s.natsLoader` is `*natsSiteLoader`.                   |

If the existing tests don't yet expose loader internals, expose via a package-private accessor `func (s *Server) natsLoaderForTest() DocumentationLoader { return s.natsLoader }` — keep it small and test-only by naming.

### `internal/server/initialize_archive_test.go` (new)

End-to-end-ish test using the fixture archive from phase 2.

| Test                                              | Asserts                                                            |
|---------------------------------------------------|--------------------------------------------------------------------|
| `TestInitialize_ArchiveSource_IndexesFixture`     | Build `Server` with `archive` + fixture path, no cache; call `Initialize(ctx)`; assert NATS index count matches expected canonical count. |
| `TestInitialize_ArchiveSource_RetrieveRootByID`   | `s.indexManager.GetNATSIndex().Get("index")` returns the README.   |
| `TestInitialize_ArchiveSource_RetrieveNestedReadme` | `Get("nested/b")` returns the nested README.                     |
| `TestInitialize_ArchiveSource_RetrieveAlias`      | `Get("overview-old")` returns the `overview` document via alias on a cold load. |
| `TestInitialize_ArchiveSource_WarmCacheRestoresAliases` | Run `Initialize` twice with the same cache dir; second call hits the cache; `Get("overview-old")` still resolves via the persisted alias map. |
| `TestInitialize_ArchiveSource_KindMismatchInvalidatesCache` | Pre-seed `nats-archive.json` with `Kind="site_html"` (or v1.0 cache from the site path), boot with archive mode; cache is treated as a miss and the loader runs. |
| `TestInitialize_ModeSwitch_SiteToArchive`         | Run with `site` first (writes `nats-site.json`), then restart with `archive` + fixture path; archive mode loads from archive, **not** from the site cache, and `nats-archive.json` is written. |
| `TestInitialize_ModeSwitch_ArchiveToSite`         | Symmetric: archive cache present, switch to `site` (use stub site loader); does not pick up archive cache. |
| `TestInitialize_ArchiveSource_OfflineNoNetwork`   | Set `DOCS_BASE_URL=http://invalid.example.invalid`; archive path still works because no HTTP is involved. |
| `TestInitialize_RefreshCacheClearsOtherMode`      | With both `nats-site.json` and `nats-archive.json` present and `RefreshCache=true` in archive mode, the site cache file is removed and the archive cache is rewritten fresh. |

`Initialize` is the entry point; tests should construct `Server` via `NewServer` so the loader-selection logic is exercised. Use `t.Setenv` and an in-process temp cache dir.

### `internal/server/tools_test.go` adjustments

If any test calls `handleRetrieveTool` with a docs ID, add a parallel case verifying that a redirect alias resolves successfully when the archive loader populated the alias map.

## Logging

Standardise startup log lines emitted from `initializeNATS`:

```
INFO Loading NATS documentation source_type=archive
INFO NATS documentation indexed count=N kind=github_archive origin=assets/nats.docs-master.zip revision=master aliases=K
```

The `kind` value comes from `SourceMetadata.Kind` so a future log-scan can distinguish runs.

## Observability counters (optional but trivial)

If `zerologLogger` is at debug level, emit one debug line per category:

```
DEBUG archive entries skipped category=bookignore  count=160
DEBUG archive entries skipped category=zh-cn       count=22
DEBUG archive entries skipped category=empty       count=26
DEBUG archive entries indexed orphans=0 canonical=187 legacy=0
```

These help phase 4 parity testing. Skip if it complicates the loader — they're not in the proposal's acceptance criteria.

## Out of scope for phase 3

- Switching the default to `archive`. That happens in phase 4.
- Remote archive download via `NATSArchiveURL`. The field is reserved; validation rejects it explicitly.
- Replacing Synadia / GitHub fetchers.

Cache-alias persistence and source-namespaced cache keys are **in scope** for phase 3 (per the review finding) — they are not safe to defer.

## Definition of done

- `go build ./...` succeeds.
- `go test ./...` passes including the new server-integration tests and the updated config property tests.
- Default config (`nats_source_type=site`) yields identical behaviour to today: no regressions in existing `server_test.go` or `tools_test.go`.
- `nats_source_type=archive` + only `nats_archive_url` set fails `Validate()` with a "reserved" error.
- Cache files are written as `nats-site.json` or `nats-archive.json`. Files from cache version `1.0` continue to load (no migration tool needed).
- Mode-switch tests confirm the cache from the other mode is never imported into the new mode.
- Running `nats-docs-mcp-server --config config-with-archive.yaml` starts cleanly using the local zip and serves `search_nats_docs` + `retrieve_nats_doc` against the indexed pages, verified manually. A second run hits the cache and the alias retrieval test (`retrieve_nats_doc` with a redirect target) still succeeds.
- The Synadia and GitHub init paths are unchanged — they still call `cache.Save` and receive identical behaviour.
