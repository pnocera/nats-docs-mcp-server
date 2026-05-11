## Pass 2 review — GO

All five required acceptance items from the previous review are addressed, the verification commands you listed pass on my machine, and I confirmed the asset/CI/release surface end-to-end.

### Verified resolutions

1. **Archive tracked, not LFS, ignore narrowed.**
   - `git ls-files --stage assets/nats.docs-master.zip` → `100644 c487b7ba0 … 0` (regular 5.5 MB blob, not LFS).
   - `git check-ignore` reports nothing on the zip; `.gitignore:43` only excludes `assets/nats.docs-master/` and now ends with a trailing newline.
   - Archive contents: 720 entries, deflate-compressed, expands to ~24 MB; opens cleanly.

2. **CI will exercise the smoke test on fresh checkouts.**
   - `.github/workflows/ci.yaml:22,57,76` all on Go 1.24.
   - `ci.yaml:36` runs `go test … ./...` (which includes `TestInitialize_DefaultArchiveAssetSmoke`) and `:39` reruns the smoke target explicitly. With the zip tracked, both succeed on a clean checkout.

3. **Release archive bundles the asset.**
   - `.goreleaser.yaml:66` adds `assets/nats.docs-master.zip` to `archives.files`.
   - Container/distroless story is documented at `README.md:76` (mount-and-set-env, or `NATS_DOCS_NATS_SOURCE_TYPE=site`) rather than bundled — acceptable for v1.

4. **`config.example.yaml` is now self-consistent and load-tested.**
   - `:13 docs_base_url`, `:21-23` archive defaults, `:35 fetch_timeout: 30` (int), `:49 max_search_results: 50`, `:87 synadia.fetch_timeout: 30` (int).
   - `internal/config/config_test.go:490 TestConfigExampleLoads` actually loads it via `LoadFromFile` and asserts the documented defaults; runs in 2 ms.

5. **Parity report meets the ≥95% bar.**
   - `work/reviews/parity-phase-4-2026-05-11.md:4` — 186/186 (100.0%) of site IDs resolve through archive IDs or aliases; `:26` — 8/8 archive spot checks have non-empty content; ID lists truncated to 20 with `+N more`.
   - Direct-only ID lists make the canonical path deltas explicit (e.g., `release-notes/whats_new` vs `release_notes/whats_new`, `object-store` vs `obj_store`), and they're all covered by `internal/server/nats_archive_loader.go:229-327`.

6. **Minor previous nits.** Trailing newline in `.gitignore`, `revision` field comment at `nats_archive_loader.go:22`, `NATSIndex()` docstring at `server.go:679-680`, smoke test's `Count() < 100` assertion at `initialize_archive_test.go:236` — all in.

### Local verification I reproduced

- `go test ./...` — ok
- `go test -race ./...` — ok
- `go test -tags property ./internal/config` — ok
- `go vet ./...` — clean
- `go build ./...` — clean (covers `scripts/parity_check.go`)
- `go test -v ./internal/server -run TestInitialize_DefaultArchiveAssetSmoke -count=1` — PASS (0.17s)
- `go test -v ./internal/config -run TestConfigExampleLoads -count=1` — PASS

GoReleaser isn't installed locally so I couldn't run `--snapshot`; CI's `goreleaser build` step will execute it.

### Non-blocking observations (file later, don't block the merge)

- **Site mode looks half-broken in the parity output.** Every site spot check in `parity-phase-4-2026-05-11.md:10-25` shows `length_ratio=0.00` with empty `site:` content while the archive returns real prose. That's a pre-existing site-loader regression (GitBook v2 markup change), not something this phase introduced — and it strengthens the case for flipping the default. Worth opening a follow-up so the `site` fallback documented at `README.md:78` doesn't silently degrade for anyone who reverts.
- **CI doesn't exercise `archives.files` packaging.** `.github/workflows/ci.yaml:83` runs `goreleaser build --snapshot --clean`, which produces binaries only — the new `assets/nats.docs-master.zip` line in `archives.files` won't be tested until the first real release tag. Consider `goreleaser release --snapshot --skip=publish --clean` if you want pre-tag verification that the tarball contains the asset.
- **`scripts/parity_check.go:85`** still sets `MaxConcurrent = 3`, which only affects the live-site loader path; harmless leftover.
- **Search top-3 overlap is 0–2/3 across all queries** in the parity report. Mostly explained by the site loader returning empty content (above), so TF-IDF on the site side is essentially title-only. Once site mode is deprecated this won't matter; until then, don't market the two modes as drop-in equivalent.

Nothing here blocks Phase 4 — ship it.
