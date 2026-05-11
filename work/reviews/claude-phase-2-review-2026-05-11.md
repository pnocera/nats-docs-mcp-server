# Phase 2 Review — Archive-Backed Ingestion

Build clean, `go vet` clean, `go test -race ./internal/{parser,index,server}/...` all pass.

## Findings

### Low (quality, non-blocking)

**L1 — `internal/server/nats_archive_loader.go:87-91` orphan inclusion sweeps meta files.**
When `includeOrphans=true`, `arch.List(strings.HasSuffix(".md"))` matches `SUMMARY.md` itself (and any other root-level `.md` such as the fixture's `notes..old.md`). Those become docs with IDs `SUMMARY`, `notes..old`. `.bookignore` does not list them, so they sail through into the index alongside real content. Not exercised by any test — `TestArchiveLoader_IncludesOrphans` only verifies `orphan` is present, never that meta files are absent. Default is `false`, so production impact is opt-in only.

**L2 — `internal/parser/gitbook.go:114` `html.UnescapeString` applies to every non-fenced line.**
`&amp;`, `&lt;`, numeric entities, etc. are decoded in prose. Fenced code is preserved, so syntax examples are fine, but raw prose entities are silently decoded. Probably what you want for GitBook docs, but worth knowing — the plan does not mention this transform.

**L3 — `internal/parser/gitbook.go:81-88` fence opener captures only the 3-char prefix.**
A 4-backtick opener (CommonMark-legal, e.g. ` ```` `) sets `fenceMarker="```"` and a 3-backtick line will then close it prematurely. NATS docs rarely use 4+ backticks, but the contract here is looser than CommonMark.

**L4 — `internal/parser/gitbook.go:32` catch-all `{%[^%]*%}` is non-greedy at `%`.**
A directive containing a `%` inside its body (e.g. `{% foo style="50%" %}`) won't match the catch-all and survives into output. Unlikely in practice; no test covers it.

**L5 — `internal/index/index.go:348-356` `Get` swallows non-not-found errors.**
Today `store.GetDocument` only ever returns the not-found error, so this is benign. If another error class is ever added there, `Get` will silently try the alias path. Reads a bit hand-wavy but not a current bug.

### Info

**I1 — Aliases not yet consumed.** `SetAliases` is defined and tested, but no caller in `internal/server/server.go` invokes it. This matches the plan's explicit deferral to phase 3 ("Out of scope for phase 2 — Wiring loaders into `Server.Initialize`").

**I2 — Test coverage gaps worth noting.** No tests for `docIDFromPath("")` / leading-slash inputs, `path.Clean(p) != p` rejection paths (e.g. `nested//a.md`), or the `{% code title="x" %}` directive preceding a fenced block. Existing fixture has `notes..old.md` but no test asserts inclusion.

### Correctness checks that passed

- Alias semantics: copy-on-set, nil-clears, canonical unchanged, missing-target → not-found. All covered.
- Locking: `Get` reads aliases under `di.mu.RLock()`; nested `store.GetDocument` takes a separate `store.mu.RLock()` — no ordering issue, no race detected with `-race`.
- `shouldSkipArchivePath` correctly drops `zh-cn/`, `.bookignore`-matched, and `legacy/` (when `!includeLegacy`).
- Redirect alias drop categorization (`aliases_dropped_legacy` vs `aliases_dropped_missing`) is correct and surfaced via `SourceMetadata.AliasesDropped`.
- Fence state machine handles directive-before-fence, directive-after-fence, directive-inside-fence, and mismatched markers correctly (per the existing tests).
- `docIDFromPath` maps `README.md → index`, `nested/b/README.md → nested/b`, and the URL builder distinguishes root from non-root correctly.
- Goldmark roundtrip test confirms preprocessed output is still valid Markdown.

## Verdict

**GO** for phase 2. None of the items above block landing — they're surface-area concerns to address either before phase 3 wires aliases into the server (L1 most likely) or as follow-ups. Recommend filing L1 (orphan meta-file filtering) as a phase-3 task before flipping `includeOrphans=true` in any default config.
