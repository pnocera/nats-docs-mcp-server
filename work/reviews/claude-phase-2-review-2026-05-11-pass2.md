## Pass-2 Review — Phase 2 Fixes

**Build:** clean. **Vet:** clean. **`go test -race ./internal/{parser,index,server}/...`:** all pass.

### Prior fixes — verified

| Item | Status | Evidence |
|---|---|---|
| L1: SUMMARY.md leaked via orphan sweep | ✅ Fixed | `nats_archive_loader.go:89` filters via `isArchiveMetadataMarkdown`; new test `TestArchiveLoader_OrphansSkipSummary` (`nats_archive_loader_test.go:100`) covers it. Same filter also re-checked in `shouldSkipArchivePath` at line 169 (belt-and-suspenders for canonical path). |
| L3: 4-backtick fence closed too early | ✅ Fixed | `repeatedFenceMarker` now returns the full run (`gitbook.go:118`); `isFenceCloser` requires `HasPrefix(trimmed, marker)` then validates remainder is empty or marker runs (`gitbook.go:91-108`). New test `TestPreprocess_FourBacktickFence` (`gitbook_test.go:108`). Asymmetric-length cases (longer closer for shorter opener) still handled correctly per CommonMark. |
| L4: catch-all stopped at `%` | ✅ Fixed | `gitbook.go:32` uses `\{\%.*?\%\}` (non-greedy `.`). New test `TestPreprocess_UnknownDirectiveContainingPercent` (`gitbook_test.go:116`). Non-greedy correctly handles multiple directives on one line. |

### Pass-2 spot checks

- **Alias `Get`/`SetAliases`** (`internal/index/index.go:344-369`): copy-on-set, nil-clears to empty map, canonical lookup unchanged, missing-target → not-found. Lock discipline: `Get` holds `di.mu.RLock()` and only takes the inner `store.mu.RLock()` (different mutex) — no reentrancy. `SetAliases` takes `di.mu.Lock()` and touches no other lock. All four alias tests pass under `-race`.
- **Archive path safety**: `normalizeArchiveDocPath` rejects empty, absolute, `..`-segmented, and non-clean paths. `unescapeArchivePath` strips backslash escapes, neutralizing `..\..` traversal attempts (becomes `....`). The follow-up `ReplaceAll(p, "\\", "/")` in `normalizeRedirectAlias`/`normalizeArchiveDocPath` is dead code (unescape removes all `\`) — cosmetic, not a bug.
- **Redirect categorization**: legacy vs missing dropped-alias logging works; `meta.AliasesDropped` count surfaces.
- **Fence asymmetry**: `````python` opener → `````` closer with text after (e.g., `````go``) correctly *not* treated as closer (info string after closing fence forbidden per CommonMark). Verified by tracing.

### Carried info items (non-blocking, unchanged from pass 1)

- **L2** `html.UnescapeString` on every non-fenced line — intentional per pass 1, still untested. Acceptable.
- **L5** `Get` swallows non-not-found errors from `store.GetDocument` — benign (only error class returned today is not-found).
- **I1** `SetAliases` not yet wired in `server.Initialize` — explicit phase-3 deferral.
- **I2** No tests for `docIDFromPath("")`, leading-slash inputs, or non-clean rejection paths.

### New low-priority observation

- **`isArchiveMetadataMarkdown` matches `SUMMARY.md` exactly** (`nats_archive_loader.go:184-186`). A nested `subdir/SUMMARY.md` would be indexed under `includeOrphans=true`. Not present in NATS docs; flagging only because the function name implies broader meta-file handling than it actually does. No action required.

### Verdict

**GO.** All three pass-1 blocking-eligible items are fixed with targeted tests. No new correctness, security, parser, alias, or locking issues found. Phase 2 is ready to land; the three carried info items remain follow-ups, not gates.
