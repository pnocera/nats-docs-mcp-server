# Phase 1 — Archive Reader and Manifest Parsing

**Outcome:** a `fetcher.Archive` type that opens `nats.docs-master.zip` safely, plus parsers for `SUMMARY.md`, `.bookignore`, and `.gitbook.yaml`. No server wiring yet. All work behind `go test`.

## Files

### New: `internal/fetcher/archive.go`

Purpose: open a local zip file (or an `io.ReaderAt` for future remote use), walk entries safely, and expose them to callers as `(path, contents)` pairs.

```go
package fetcher

import (
    "archive/zip"
    "context"
    "errors"
    "fmt"
    "io"
    "io/fs"
    "os"
    "path"
    "strings"
)

// ArchiveLimits caps untrusted-input growth.
type ArchiveLimits struct {
    MaxFiles            int   // default 5000
    MaxUncompressedSize int64 // default 256 MiB
    MaxFileSize         int64 // default 8 MiB
}

func DefaultArchiveLimits() ArchiveLimits {
    return ArchiveLimits{MaxFiles: 5000, MaxUncompressedSize: 256 << 20, MaxFileSize: 8 << 20}
}

// ArchiveEntry is a single file inside the archive, already read into memory.
// Paths are normalised to forward slashes and are relative to the archive root
// (the top-level directory like "nats.docs-master/" is stripped).
type ArchiveEntry struct {
    Path    string // e.g. "nats-concepts/jetstream/streams.md"
    Content []byte
}

// Archive is an in-memory snapshot of an opened zip after safety checks.
type Archive struct {
    Root    string // e.g. "nats.docs-master"
    Entries map[string]ArchiveEntry // keyed by Path
}

// OpenLocalArchive opens a zip at path, validates it, and returns an Archive.
func OpenLocalArchive(ctx context.Context, archivePath string, limits ArchiveLimits) (*Archive, error)

// openZip is shared between local and (future) remote callers.
func openZip(ctx context.Context, r *zip.Reader, limits ArchiveLimits) (*Archive, error)

// Get returns an entry by relative path, or false.
func (a *Archive) Get(p string) (ArchiveEntry, bool)

// List returns relative paths matching a predicate. Used by SUMMARY discovery
// and orphan detection in phase 2.
func (a *Archive) List(pred func(string) bool) []string
```

Safety rules enforced inside `openZip`:

1. Normalize backslashes to forward slashes, then reject entries whose `Name` is absolute (starts with `/`).
2. Reject entries whose name contains NUL bytes.
3. Reject entries whose cleaned path has any segment equal to `..`. Implementation:
   ```go
   clean := path.Clean(name)
   for _, seg := range strings.Split(clean, "/") {
       if seg == ".." {
           return errTraversal
       }
   }
   ```
   This does **not** use `strings.Contains(..., "..")` — that would falsely reject legal names like `notes..old.md`.
4. Track running file count against `MaxFiles` and uncompressed total against `MaxUncompressedSize`.
5. Reject any single entry larger than `MaxFileSize`.
6. Skip directories (`fi.IsDir()`); never write to disk.
7. Skip symlink and irregular entries (`fi.Mode() & fs.ModeType != 0`).
8. Identify the archive root as the segment before the first `/` in the first regular entry's path. Every subsequent regular entry must have the same root segment; otherwise error. (Longest-common-prefix is overkill here — the GitHub-zip convention guarantees a single top-level directory.) The stored `Path` is the suffix below that root.

Return an `Archive` populated only with regular files. Bookkeeping for what was skipped is logged at the caller boundary, not inside this package (keep `fetcher` log-free aside from the existing zerolog instances).

### New: `internal/parser/bookignore.go`

Parses the simple `.bookignore` format used by GitBook:

- one pattern per line
- blank lines and lines starting with `#` ignored
- trailing `/` means "match a directory and everything under it"
- otherwise, exact filename match anywhere in the path

```go
package parser

type BookIgnore struct {
    dirPrefixes []string // "_book/", "docs/", ...
    exactNames  []string // "Makefile", "building_the_book.md", ...
}

func ParseBookIgnore(content []byte) *BookIgnore
func (b *BookIgnore) Match(relPath string) bool
```

`Match` should return true if any `dirPrefixes` is a prefix of `relPath` or any `exactNames` equals the basename. Cover the actual archive's patterns: `_book/`, `_docs/`, `_examples/`, `_tools/`, `docs/`, plus `Makefile`, `building_the_book.md`, `.gitignore`, `.bookignore`.

### New: `internal/parser/summary.go`

Parses `SUMMARY.md` into an ordered list of canonical pages.

```go
package parser

type SummaryLink struct {
    Title string // visible link text, e.g. "Pub/Sub Walkthrough"
    Path  string // archive-relative .md path, e.g. "nats-concepts/.../pubsub_walkthrough.md"
}

func ParseSummary(content []byte) ([]SummaryLink, error)
```

Implementation details:

- Use Goldmark to parse the markdown, walk for `*ast.Link` nodes, capture link text and destination.
- Only keep links whose destination ends in `.md` (skip section headers and external links).
- Unescape GitBook backslash escapes in paths: `\_` -> `_`. The proposal lists `nats\_cli` as the example — apply a single pass that replaces `\\_` with `_` and `\\` followed by any other char with that char (be conservative; only `_` is observed in this archive).
- Strip a leading `./` if present.
- Preserve order (it determines canonical navigation order, which we may use for stable IDs later).
- De-duplicate while preserving first occurrence.

Reject malformed input only if the file cannot be parsed; missing links produce an empty slice without error.

### New: `internal/parser/gitbook_redirects.go`

Parses `.gitbook.yaml`'s `redirects:` block. Tolerate the rest of the file (we don't care about `root:` or `structure:`).

```go
package parser

type Redirect struct {
    From string // e.g. "nats-tools/nas"
    To   string // e.g. "legacy/nas/README.md" (cleaned, no leading "./")
}

func ParseGitBookRedirects(content []byte) ([]Redirect, error)
```

Use `gopkg.in/yaml.v3` (already in `go.sum` indirectly — promote to a direct require). Unmarshal into:

```go
type rawGitBookConfig struct {
    Redirects map[string]string `yaml:"redirects"`
}
```

Then flatten to a slice, stripping leading `./` from `To`. Preserve insertion order is not possible from a YAML map; sort the result by `From` so behaviour is deterministic for tests.

If `.gitbook.yaml` is absent (the loader will check), the caller passes `nil` content and gets back `(nil, nil)`.

## Tests

All in new file `internal/fetcher/archive_test.go` and per-parser `_test.go` files.

### `archive_test.go`

Build fixture zips in-memory using `archive/zip.NewWriter` so tests are hermetic. No on-disk fixtures.

| Test                                     | What it asserts                                                            |
|------------------------------------------|----------------------------------------------------------------------------|
| `TestOpenLocalArchive_HappyPath`         | Reads a 3-file zip wrapped in `repo-master/`, returns entries keyed without the prefix. |
| `TestOpenLocalArchive_RejectsAbsolute`   | Entry named `/etc/passwd` -> error.                                        |
| `TestOpenLocalArchive_RejectsTraversal`  | Entry named `repo-master/../etc/passwd` -> error.                          |
| `TestOpenLocalArchive_AllowsDoubleDotInBasename` | Entry named `repo-master/notes..old.md` -> accepted (segment-based check, not substring). |
| `TestOpenLocalArchive_RejectsBackslashAbsolute` | Entry name `\windows\path` -> error after slash normalisation.            |
| `TestOpenLocalArchive_RejectsMixedRoots` | First entry under `repo-master/`, second under `other/` -> error.          |
| `TestOpenLocalArchive_RejectsNul`        | Entry name containing `\x00` -> error.                                     |
| `TestOpenLocalArchive_FileCountLimit`    | Limits = 2 files; zip has 3 -> error after the third.                      |
| `TestOpenLocalArchive_TotalSizeLimit`    | Limits = 100 B total; zip has 3 × 50 B files -> error.                     |
| `TestOpenLocalArchive_PerFileSizeLimit`  | Limits per-file = 50 B; zip has a 100 B file -> error.                     |
| `TestOpenLocalArchive_SkipsDirs`         | Directory entries don't appear in `Entries`.                               |
| `TestOpenLocalArchive_SkipsSymlinks`     | Symlink entries don't appear in `Entries`.                                 |
| `TestOpenLocalArchive_MissingRoot`       | Entries with no common prefix -> error.                                    |
| `TestArchive_Get`                        | Round-trips a stored entry.                                                |
| `TestArchive_List`                       | Predicate filters as expected.                                             |

### `bookignore_test.go`

| Test                                | What it asserts                                                            |
|-------------------------------------|----------------------------------------------------------------------------|
| `TestParseBookIgnore_Comments`      | `#` lines and blanks ignored.                                              |
| `TestParseBookIgnore_DirPrefixes`   | `_book/` matches `_book/foo.html` and `_book/sub/x.md`.                    |
| `TestParseBookIgnore_ExactNames`    | `Makefile` matches `Makefile` and `subdir/Makefile`.                       |
| `TestParseBookIgnore_NoFalsePositives` | `docs/` matches `docs/x` but not `running-a-nats-service/x`.            |

### `summary_test.go`

| Test                                 | What it asserts                                                           |
|--------------------------------------|---------------------------------------------------------------------------|
| `TestParseSummary_BasicLinks`        | `* [Welcome](README.md)` -> `{Title:"Welcome", Path:"README.md"}`.        |
| `TestParseSummary_EscapedUnderscore` | `[X](release\_notes/whats\_new.md)` -> `release_notes/whats_new.md`.      |
| `TestParseSummary_StripsLeadingDot`  | `./README.md` -> `README.md`.                                             |
| `TestParseSummary_PreservesOrder`    | Order matches input.                                                      |
| `TestParseSummary_DedupesByPath`     | Same path twice -> one entry, first title kept.                           |
| `TestParseSummary_SkipsExternal`     | `https://...` and `#anchor` links dropped.                                |
| `TestParseSummary_SkipsNonMD`        | `something.svg` dropped.                                                  |
| `TestParseSummary_RealArchiveExcerpt`| Run against the first ~30 lines of the real `SUMMARY.md` (paste as a Go string literal). Assert no errors, ≥10 links, first link is `README.md`. |

### `gitbook_redirects_test.go`

| Test                                   | What it asserts                                                          |
|----------------------------------------|--------------------------------------------------------------------------|
| `TestParseGitBookRedirects_Basic`      | One redirect parses to the expected struct.                              |
| `TestParseGitBookRedirects_StripDot`   | `./legacy/foo.md` becomes `legacy/foo.md`.                               |
| `TestParseGitBookRedirects_Empty`      | `nil` input -> `(nil, nil)`.                                             |
| `TestParseGitBookRedirects_NoBlock`    | File with `root:` but no `redirects:` -> `(nil, nil)`.                   |
| `TestParseGitBookRedirects_SortedOut`  | Output sorted by `From` (since map order is random).                     |
| `TestParseGitBookRedirects_RealSample` | The first ~10 entries of the real `.gitbook.yaml` parse without error.   |

## Dependency change

Promote `gopkg.in/yaml.v3` from indirect to direct:

```
require (
    ...
    gopkg.in/yaml.v3 v3.0.1
)
```

Run `go mod tidy` after adding the import in `gitbook_redirects.go`.

## Out of scope for phase 1

- Anything that produces `index.Document`.
- GitBook preprocessor (phase 2).
- HTTP archive downloads (deferred; the interface in `Archive` already supports it via a future `OpenRemoteArchive`).
- Any change to `internal/server` or `internal/config`.

## Definition of done

- `go build ./...` succeeds.
- `go test ./internal/fetcher/... ./internal/parser/...` passes including all tests listed above.
- No new imports of `internal/parser` or `internal/fetcher` outside their own tests.
- `go mod tidy` is clean.
