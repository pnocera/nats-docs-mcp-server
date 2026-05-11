Looking at the second-pass changes after fixes.

**Verification commands run:**
- `go build ./...` — clean
- `go vet ./...` — clean
- `go test ./internal/fetcher/... ./internal/parser/...` — passes

**Status of prior findings:**

1. **`internal/parser/gitbook_redirects.go:60-75` — redirect traversal (prior Low)** — **FIXED.** `normalizeRedirectTo` now calls `hasParentSegment` both before and after `path.Clean`, and `TestParseGitBookRedirects_RejectsTraversal` pins it. Traversal payloads like `../../etc/passwd` and `foo/../../etc/passwd` are rejected. The pre-clean check also blocks pathological inputs where `path.Clean` would collapse traversal segments. Belt-and-suspenders is appropriate here.

2. **`internal/fetcher/archive.go:116-120` — underflow in size compare (prior Low)** — **FIXED.** A new `if fileSize > limits.MaxUncompressedSize` guard (lines 116–118) runs before the subtraction at line 119, so `MaxUncompressedSize - fileSize` is non-negative and the error message at line 117 now correctly attributes the failure to the single file rather than the running total.

3. **`internal/parser/summary.go:70-90` — `unescapeGitBookPath` broader than spec (prior Low)** — **Not changed**, but `TestParseSummary_EscapedUnderscore` pins the intended `\_` → `_` behavior. Style-only; behavior is intentionally lenient. As a side effect, Windows-style backslash traversal in redirect values (`..\..\etc\passwd`) gets neutralized into `......etcpasswd` by the backslash-consuming unescape — incidental but not exploitable.

**New review of the changes themselves (looking for blocking issues only):**

- `normalizeRedirectTo` does not reject absolute paths (e.g. `/etc/passwd` survives, since only `..` is filtered). Not blocking — `Archive.Get` does map lookup, not filesystem access, so an absolute target just produces a benign cache miss. Worth flagging only as info; the archive-name path correctly rejects absolutes for entries.
- `normalizeRedirectTo` does not check for NUL bytes, unlike `normalizeArchiveName`. Same reasoning — not exploitable, just inconsistent.
- `openZip` skips symlinks/devices via `info.Mode()&fs.ModeType != 0` after the `IsDir` short-circuit; symlinks do not count against limits, but since contents are never read, that's fine. Covered by `TestOpenLocalArchive_SkipsSymlinks`.
- Root-directory detection ignores skipped entries (dirs, symlinks) when establishing `root`, which is correct: the first regular file wins. Multi-root and missing-root cases both have tests.
- `Archive.Get` cleans `\` → `/` and strips `./` before map lookup, matching the entry-key normalization done at ingest. Consistent.
- `ParseGitBookRedirects` returns deterministic sorted output; empty/no-block paths return `(nil, nil)`. Covered.
- `BookIgnore.Match` correctly anchors `dirPrefixes` at the start of the rel path (no `docsy/...` false positive). Covered.

No blocking correctness or security issues remain.

## Verdict

**GO** — the prior redirect-traversal Low is properly fixed (with pre- and post-clean checks plus a dedicated test), the archive size-compare underflow is fixed, and no new blocking issues were introduced. The remaining nits (`unescapeGitBookPath` looseness, absolute/NUL not rejected in redirect `To`) are non-exploitable and can be deferred — none of them gate phase 2.
