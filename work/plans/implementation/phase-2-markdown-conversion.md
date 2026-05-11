# Phase 2 — Markdown Document Conversion

**Outcome:** given an `Archive` plus the parsed `SUMMARY.md` / `.bookignore` / `.gitbook.yaml`, produce `[]*index.Document` ready for `IndexNATS`, plus an alias map for redirect resolution. GitBook directives are stripped before Goldmark sees them. Still no server wiring — phase 3.

Depends on phase 1.

## Files

### New: `internal/parser/gitbook.go`

A text-level preprocessor that runs once over each Markdown file's bytes before `ParseMarkdown`. Pure function, no AST.

```go
package parser

// PreprocessGitBook rewrites GitBook directive syntax into plain Markdown so
// that Goldmark produces clean sections. Fenced code blocks are passed through
// untouched.
func PreprocessGitBook(src []byte) []byte
```

Transformations (apply in order; each is a regex `ReplaceAllString`-style pass):

| Input directive                                | Output                              |
|------------------------------------------------|-------------------------------------|
| `{% hint style="info" %}`                      | `Info:`                             |
| `{% hint style="warning" %}`                   | `Warning:`                          |
| `{% hint style="danger" %}`                    | `Warning:`                          |
| `{% hint style="success" %}`                   | `Note:`                             |
| `{% hint %}` (no style)                        | `Note:`                             |
| `{% endhint %}`                                | (removed)                           |
| `{% tabs %}` / `{% endtabs %}`                 | (removed)                           |
| `{% tab title="Go" %}`                         | `### Go`                            |
| `{% endtab %}`                                 | (removed)                           |
| `{% embed url="https://x" %}`                  | `Embedded: https://x`               |
| `{% endembed %}`                               | (removed)                           |
| `{% code title="x" %}` / `{% endcode %}`       | (removed; the fenced block follows) |
| Any other `{% ... %}` directive                | (removed)                           |
| `<figure>` ... `</figure>` wrapper             | inner text preserved (strip tags)   |
| `<img ... alt="X" ...>`                        | `X`                                 |
| `<figcaption>X</figcaption>`                   | `X`                                 |

Implementation constraint: do not rewrite inside fenced code blocks. **Do not** split-on-fence by index parity — that breaks when the document begins or ends mid-fence and is easy to get backwards. Use a line-oriented state machine:

```go
func PreprocessGitBook(src []byte) []byte {
    var out bytes.Buffer
    inFence := false
    var fenceMarker string // "```" or "~~~"
    for _, line := range splitKeepNewlines(src) {
        trimmed := strings.TrimLeft(line, " \t")
        if !inFence {
            if marker, ok := fenceOpener(trimmed); ok {
                inFence = true
                fenceMarker = marker
                out.WriteString(line)
                continue
            }
            out.WriteString(applyDirectiveRewrites(line))
            continue
        }
        // inside a fence — passthrough byte-for-byte
        out.WriteString(line)
        if strings.HasPrefix(trimmed, fenceMarker) && isFenceCloser(trimmed, fenceMarker) {
            inFence = false
            fenceMarker = ""
        }
    }
    return out.Bytes()
}
```

Notes:

- Support both ` ``` ` and `~~~` fence styles (cheap and matches CommonMark).
- A closing fence must use the same marker as the opener; mismatched markers inside an open fence are treated as content.
- `applyDirectiveRewrites` runs the regex passes from the table above against a single line. Each pattern is single-line so multi-line directives (e.g. `{% hint %}` ... `{% endhint %}` separated by blank lines) become two independent rewrites, one per line — which is exactly what we want for Markdown output.
- The complete set of rewrites is short — store them as a package-level `[]struct{re *regexp.Regexp; repl string}` initialized once.

### New: `internal/server/loader.go`

Defines the loader interface and shared types. Lives in `server` because both archive and site implementations need access to `config.Config` and `index.Document` and the existing logger.

```go
package server

import (
    "context"

    "github.com/j4ng5y/nats-docs-mcp-server/internal/index"
)

// DocumentationLoader produces ready-to-index documents from a single
// documentation source.
type DocumentationLoader interface {
    Load(ctx context.Context) ([]*index.Document, SourceMetadata, error)
}

// SourceMetadata describes where the documents came from. It is stored
// alongside the cache and emitted in startup logs.
type SourceMetadata struct {
    Source           string            // "nats", "syncp", "github"
    Kind             string            // "site_html" or "github_archive"
    SourceURL        string            // public base URL (for cache + URL building)
    Origin           string            // archive path or sitemap URL
    Revision         string            // optional: branch or commit SHA
    Aliases          map[string]string // alias doc-id -> canonical doc-id, may be nil
    AliasesDropped   int               // count of redirects skipped because target wasn't indexed
    DocumentIDs     []string           // canonical doc IDs in canonical order
}
```

No behaviour here — types only.

### New: `internal/server/nats_archive_loader.go`

The actual converter. Implements `DocumentationLoader`.

```go
package server

import (
    "context"
    "fmt"
    "log/slog"
    "path"
    "sort"
    "strings"
    "time"

    "github.com/j4ng5y/nats-docs-mcp-server/internal/fetcher"
    "github.com/j4ng5y/nats-docs-mcp-server/internal/index"
    "github.com/j4ng5y/nats-docs-mcp-server/internal/parser"
)

type natsArchiveLoader struct {
    archivePath    string
    docsBaseURL    string
    includeOrphans bool
    includeLegacy  bool
    limits         fetcher.ArchiveLimits
    logger         *slog.Logger
}

func newNATSArchiveLoader(cfg natsArchiveConfig, logger *slog.Logger) *natsArchiveLoader
func (l *natsArchiveLoader) Load(ctx context.Context) ([]*index.Document, SourceMetadata, error)
```

`natsArchiveConfig` is a small struct populated from `*config.Config` (defined here, not in `internal/config`, to avoid leaking archive-only fields outside server wiring):

```go
type natsArchiveConfig struct {
    ArchivePath    string
    DocsBaseURL    string
    IncludeOrphans bool
    IncludeLegacy  bool
}
```

#### Load algorithm

1. `arch, err := fetcher.OpenLocalArchive(ctx, l.archivePath, l.limits)`.
2. Look up `.bookignore`, `SUMMARY.md`, `.gitbook.yaml` from `arch.Get`. `SUMMARY.md` is required; the others are optional.
3. Build the canonical set:
   - Start from `parser.ParseSummary` results.
   - If `includeOrphans`, append every `*.md` in the archive not matched by `BookIgnore` and not under `zh-cn/`, deduped against the summary set.
   - Always include `README.md` even if `SUMMARY.md` somehow omits it.
4. Filter:
   - Drop paths matched by `BookIgnore`.
   - Drop paths under `zh-cn/`.
   - Drop entries whose contents are empty after `TrimSpace`.
   - If `!includeLegacy`, drop paths under `legacy/`.
5. Convert each surviving entry to an `index.Document`:
   - `id := docIDFromPath(p)` (helper below).
   - `url := joinURL(l.docsBaseURL, urlFromPath(p))`.
   - `pre := parser.PreprocessGitBook(entry.Content)`.
   - `doc, err := parser.ParseMarkdown(pre, p)` — on error, log + skip.
   - Use the existing `extractContent` / `convertSections` helpers from `server.go` (move to `loader.go` if a cycle forces it; otherwise reference them directly since loader is in the same package).
6. Build aliases from `.gitbook.yaml`:
   - For each redirect, normalise `From` like a URL path (trim leading/trailing `/`, unescape any backslash escapes) — `From` is already a slash-separated path, not a `.md` file.
   - `to := docIDFromPath(redirect.To)`.
   - Only emit the alias if `to` is in the indexed document set built in steps 1–5. Otherwise log at info level with category `aliases_dropped_legacy` (when `To` is under `legacy/`) or `aliases_dropped_missing` (other cases) and skip the entry. The loader returns counts of both categories in `SourceMetadata` for logging visibility (`AliasesDropped int`).
   - **Behavioural consequence:** when `includeLegacy=false` (default), every redirect that points under `legacy/` is dropped, because its target was filtered out in step 4. This is the intended policy — see the README "Key design decisions" section. Operators who want legacy redirects to resolve must set `nats_archive_include_legacy=true`, which both keeps the legacy targets in the index and lets their aliases resolve.
   - One canonical redirect in the fixture (`overview-old -> overview.md`) and one legacy redirect (`nats-tools/nas -> legacy/nas/README.md`) cover both paths in tests.
7. Return `(docs, SourceMetadata{...}, nil)`. Sort `DocumentIDs` for log stability.

#### Path → URL/ID helpers

Local to `nats_archive_loader.go`:

```go
// urlFromPath converts an archive-relative .md path to a docs.nats.io URL path.
//   README.md                                 -> "/"
//   overview.md                               -> "/overview"
//   nats-concepts/jetstream/streams.md        -> "/nats-concepts/jetstream/streams"
//   nats-concepts/jetstream/README.md         -> "/nats-concepts/jetstream"
func urlFromPath(p string) string

// docIDFromPath returns the canonical index ID for an archive-relative path.
// Mirrors urlFromPath but uses "index" for the root and never has a leading slash.
//   README.md                          -> "index"
//   overview.md                        -> "overview"
//   nats-concepts/jetstream/README.md  -> "nats-concepts/jetstream"
//   nats-concepts/jetstream/streams.md -> "nats-concepts/jetstream/streams"
func docIDFromPath(p string) string
```

Both must accept and normalise backslash escapes (`release\_notes/whats\_new.md`) defensively, even though `ParseSummary` already cleans them — defence in depth.

Both must reject `path.Clean(p) != p` inputs (no `..` slipped through phase 1 — defensive).

#### Notes on extractContent / convertSections

`extractContent` and `convertSections` already exist in `internal/server/server.go` (lines 629–649). Since `nats_archive_loader.go` is in the same package, call them directly. No move needed.

### New: `internal/server/nats_site_loader.go`

Wrap the existing inline HTML path into a `DocumentationLoader`. This is a pure refactor of `initializeNATS` lines 250–278 in `server.go` — no behavioural change.

```go
type natsSiteLoader struct {
    fetcher     *fetcher.MultiSourceFetcher
    docsBaseURL string
    logger      *slog.Logger
}

func (l *natsSiteLoader) Load(ctx context.Context) ([]*index.Document, SourceMetadata, error)
```

The body is the existing fetch + parse loop, returning `SourceMetadata{Source: "nats", Kind: "site_html", SourceURL: l.docsBaseURL, Origin: l.docsBaseURL}`.

Phase 3 will route `initializeNATS` to one of these. For phase 2, just compile the file so the loader interface is real, with one test that verifies it compiles and returns valid metadata when called with a stub fetcher (or skip the test if a stub is heavy — at minimum, `_ = (DocumentationLoader)(&natsSiteLoader{})` to lock the interface).

### Modified: `internal/index/index.go`

Add an alias table to `DocumentationIndex` and consult it in `Get`. **Strictly additive — keep `searchIndex` as the field name and keep `mu` covering its current scope (`Index`, `Get`, `Search`, `Count`, `ExportDocuments`, `ImportDocuments`).** Do not change locking semantics for existing methods.

```go
type DocumentationIndex struct {
    store       *DocumentStore
    searchIndex *SearchIndex
    aliases     map[string]string // alias id -> canonical id; guarded by mu
    mu          sync.RWMutex
}

func NewDocumentationIndex() *DocumentationIndex {
    return &DocumentationIndex{
        store:       NewDocumentStore(),
        searchIndex: NewSearchIndex(),
        aliases:     map[string]string{},
    }
}

// SetAliases replaces the alias map. The input is copied so callers cannot
// mutate the index after setting. Pass nil or empty to clear.
func (di *DocumentationIndex) SetAliases(aliases map[string]string) {
    di.mu.Lock()
    defer di.mu.Unlock()
    next := make(map[string]string, len(aliases))
    for k, v := range aliases {
        next[k] = v
    }
    di.aliases = next
}

// Get retrieves a document by its canonical ID, or by an alias whose target
// is indexed. Behaviour for canonical IDs is unchanged.
func (di *DocumentationIndex) Get(id string) (*Document, error) {
    di.mu.RLock()
    defer di.mu.RUnlock()

    if doc, err := di.store.GetDocument(id); err == nil {
        return doc, nil
    }
    canonical, ok := di.aliases[id]
    if !ok {
        return nil, fmt.Errorf("document not found: %s", id)
    }
    return di.store.GetDocument(canonical)
}
```

Notes:

- `Get` still takes `mu.RLock()` exactly as today; the alias lookup happens under the same read lock so there's no new lock ordering to reason about.
- `SetAliases` takes `mu.Lock()`, sharing the existing write lock with `Index` / `ImportDocuments`. Callers should call it after the corresponding `IndexX` call so aliases never point at not-yet-indexed targets.
- The map is initialised in the constructor — `Get` does not need to nil-check.

`SetAliases` will be called by the server after `IndexNATS` (and after `ImportDocuments` on cache hit) in phase 3. Phase 2 just adds the API and tests.

## Tests

### `internal/parser/gitbook_test.go`

| Test                                   | Asserts                                                                |
|----------------------------------------|------------------------------------------------------------------------|
| `TestPreprocess_HintInfo`              | `{% hint style="info" %}foo{% endhint %}` -> `Info:\nfoo\n`.           |
| `TestPreprocess_HintWarning`           | `warning` -> `Warning:`.                                               |
| `TestPreprocess_HintSuccess`           | `success` -> `Note:`.                                                  |
| `TestPreprocess_TabsTitle`             | `{% tab title="Go" %}body{% endtab %}` -> `### Go\nbody\n`.            |
| `TestPreprocess_TabsEmbedded`          | `{% tabs %}` and `{% endtabs %}` removed cleanly.                      |
| `TestPreprocess_Embed`                 | `{% embed url="https://x" %}` -> `Embedded: https://x`.                |
| `TestPreprocess_UnknownDirective`      | `{% foo bar=baz %}` removed (catch-all).                               |
| `TestPreprocess_CodeFenceUntouched`    | Directive syntax inside ``` ```go ... ``` ``` survives byte-for-byte.  |
| `TestPreprocess_TildeFenceUntouched`   | Same, but with `~~~` fences.                                           |
| `TestPreprocess_DirectiveBeforeFence`  | `{% hint %}` on a line before a fenced block is rewritten; fence content stays intact. |
| `TestPreprocess_DirectiveAfterFence`   | `{% hint %}` on a line after a fenced block is rewritten.              |
| `TestPreprocess_DirectiveInsideFence`  | `{% hint %}` literally inside a fenced block is preserved.             |
| `TestPreprocess_MismatchedFenceMarker` | A `~~~` line inside an open ``` block is treated as content, not as a closer. |
| `TestPreprocess_HTMLFigure`            | `<figure><img alt="A logo" src="x"/><figcaption>caption</figcaption></figure>` -> `A logo\ncaption`. |
| `TestPreprocess_GoldmarkRoundtrip`     | After preprocessing, `parser.ParseMarkdown` produces ≥1 section with no errors for each fixture. |

### `internal/server/nats_archive_loader_test.go`

Use the fixture zip built in-memory by the test helper. Targets the full happy path.

| Test                                    | Asserts                                                                 |
|-----------------------------------------|-------------------------------------------------------------------------|
| `TestArchiveLoader_LoadsCanonicalPages` | All `SUMMARY.md`-linked pages indexed; URLs point at `docsBaseURL`.     |
| `TestArchiveLoader_RootReadmeIsIndex`   | `README.md` -> ID `index`, URL `https://docs.nats.io/`.                 |
| `TestArchiveLoader_NestedReadme`        | `nested/b/README.md` -> ID `nested/b`, URL `.../nested/b`.              |
| `TestArchiveLoader_SkipsBookIgnored`    | No documents from `docs/` or `_examples/`.                              |
| `TestArchiveLoader_SkipsZhCN`           | `zh-cn/placeholder.md` not indexed even though it exists.               |
| `TestArchiveLoader_SkipsEmpty`          | An empty `.md` is skipped.                                              |
| `TestArchiveLoader_SkipsOrphansByDefault` | `orphan.md` not in `SUMMARY.md` is not indexed when `includeOrphans=false`. |
| `TestArchiveLoader_IncludesOrphans`     | Same archive with `includeOrphans=true` -> `orphan.md` indexed.         |
| `TestArchiveLoader_CanonicalRedirectAlias` | A redirect whose target is a canonical (non-legacy) page emits an alias: e.g. `overview-old -> overview` is in `Aliases`. |
| `TestArchiveLoader_LegacyRedirectDroppedByDefault` | A redirect to `legacy/nas/README.md` is **not** in `Aliases` when `includeLegacy=false`. `SourceMetadata.AliasesDropped > 0`. |
| `TestArchiveLoader_LegacyRedirectResolvesWhenIncluded` | Same fixture with `includeLegacy=true`: the legacy page is indexed AND the alias `nats-tools/nas -> legacy/nas` appears. |
| `TestArchiveLoader_PreprocessApplied`   | A page using `{% hint %}` indexes without GitBook directives appearing in any section's `Content`. |
| `TestArchiveLoader_MissingSummary`      | Returns error if `SUMMARY.md` is absent.                                |
| `TestArchiveLoader_MissingArchive`      | Returns error if the archive path doesn't exist.                        |

### `internal/index/index_test.go` additions

| Test                                    | Asserts                                                                 |
|-----------------------------------------|-------------------------------------------------------------------------|
| `TestIndex_GetWithAlias`                | After `SetAliases({"old": "new"})` and indexing `new`, `Get("old")` returns the `new` document. |
| `TestIndex_GetAliasMissingTarget`       | Alias whose target is not indexed -> `Get` returns not-found error.     |
| `TestIndex_SetAliasesNilClears`         | `SetAliases(nil)` removes previous aliases.                             |
| `TestIndex_SetAliasesCopiesInput`       | Mutating the map passed to `SetAliases` after the call does not affect `Get`. |
| `TestIndex_CanonicalGetUnchanged`       | Indexing a doc and calling `Get` on its canonical ID returns it, with no alias map set (regression). |

## Fixture builder

Add `internal/server/fixture_test.go` (test-only, not in production builds):

```go
func buildFixtureArchive(t *testing.T) string {
    t.Helper()
    dir := t.TempDir()
    p := filepath.Join(dir, "nats-fixture.zip")
    f, _ := os.Create(p)
    defer f.Close()
    w := zip.NewWriter(f)
    write := func(name, body string) {
        h, _ := w.Create("nats-fixture/" + name)
        h.Write([]byte(body))
    }
    write("SUMMARY.md", "# Table of contents\n\n* [Welcome](README.md)\n* [Overview](overview.md)\n* [A](nested/a.md)\n* [B](nested/b/README.md)\n")
    write("README.md", "# Welcome\n\nRoot page.\n")
    write("overview.md", "# Overview\n\n{% hint style=\"info\" %}\nBe careful.\n{% endhint %}\n")
    write("nested/a.md", "# A\n\n{% tabs %}{% tab title=\"Go\" %}\nGo body\n{% endtab %}{% endtabs %}\n")
    write("nested/b/README.md", "# Nested B\n")
    write(".bookignore", "_book/\n_docs/\n_examples/\n_tools/\ndocs/\n\nMakefile\nbuilding_the_book.md\n")
    write(".gitbook.yaml", "redirects:\n  nats-tools/nas: ./legacy/nas/README.md\n  overview-old: ./overview.md\n")
    write("legacy/nas/README.md", "# Legacy NAS\n")
    write("zh-cn/placeholder.md", "")
    write("docs/generated.html", "<html/>")
    write("_examples/sample.html", "<html/>")
    write("orphan.md", "# Orphan\n")
    w.Close()
    return p
}
```

## Out of scope for phase 2

- Wiring loaders into `Server.Initialize`. That's phase 3.
- Remote archive downloads.
- Changes to the cache or config schema.

## Definition of done

- `go build ./...` succeeds.
- `go test ./internal/parser/... ./internal/index/... ./internal/server/...` passes including all tests listed above.
- A new tests-only fixture builder exists; no binary fixtures checked in.
- `DocumentationLoader` interface compiles and is implemented by both `natsArchiveLoader` and `natsSiteLoader`.
- `DocumentationIndex.Get` returns the same results as before for every existing test (regression check).
