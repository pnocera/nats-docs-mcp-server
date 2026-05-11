Findings collected. Writing review now.

**NOT GO** — there are blocking packaging and CI issues that must be fixed before this phase can land.

## Blocking findings

### 1. Default archive asset is gitignored and untracked — Phase 4 plan said to check it in
- `.gitignore:43` adds `assets/nats.docs-master.zip` and `assets/nats.docs-master/`.
- `git ls-files -- assets` returns empty; `git check-ignore -v` confirms it is excluded.
- `work/plans/implementation/phase-4-parity-and-cutover.md:71-72` is explicit: *"Add `assets/nats.docs-master.zip` to the repo … check it in for v1 so offline tests stay hermetic."*
- Local `go test ./...` passes only because you have the 5.7 MB zip on disk. A fresh `git clone` would not.

Fix: either remove that `.gitignore` line and `git add -f assets/nats.docs-master.zip` (5.5 MB plain commit is what the plan called for), or wire a `make fetch-archive` / `before:hook` step that downloads a pinned commit before tests run. The plan recommended the former.

### 2. CI will fail — no step downloads the archive, smoke test `t.Fatalf`s if missing
- `.github/workflows/ci.yaml:36` runs `go test -v -race ./...`, which will execute `TestInitialize_DefaultArchiveAssetSmoke`.
- `.github/workflows/ci.yaml:38-39` also adds a dedicated smoke step.
- `internal/server/initialize_archive_test.go:222-224`:
  ```go
  if _, err := os.Stat(archivePath); err != nil {
      t.Fatalf("default archive asset is required for smoke test: %v", err)
  }
  ```
- The default GitHub checkout will not contain the gitignored zip, so both CI steps will hard-fail. The "Verification run locally" doesn't catch this because the file exists on your disk.
- `.goreleaser.yaml:9` *also* runs `go test -v ./internal/...` as a `before:` hook — so releases will also break.

Fix: do (1), or change the smoke to `t.Skip` when missing AND have CI fetch/cache the archive. `t.Skip` alone hides regressions, so prefer (1) + keeping `t.Fatalf` as a hard signal.

### 3. Released binaries don't ship the archive
- `.goreleaser.yaml:60-66` archives only `README.md`, `LICENSE`, `config.example.yaml`. The default config now points at `assets/nats.docs-master.zip`, which won't exist in the tarballs/zips.
- Users who `go install` or download a release artifact and then run with the default config will fail at startup with "archive_path … not found" — a hard regression from the previous default of `site` (which worked out of the box).

Fix: add `assets/nats.docs-master.zip` to the `archives.files:` list, and (separately) decide what to do for the `kos:` container image — distroless won't contain `assets/` unless explicitly added. Either bundle into the image or document that container users must mount/override `nats.archive_path`.

## Should-fix before shipping

### 4. Default `NATSArchivePath` is a bare relative path — fragile for any cwd other than the repo root
- `internal/config/config.go:82` defaults to `"assets/nats.docs-master.zip"`.
- Once the zip ships inside the release tarball, users who extract and `cd elsewhere && nats-docs-mcp-server --config /etc/...` will get a startup error.
- Options: resolve relative to the binary's directory at startup, document the limitation in README, or require an absolute path via env when installed. At minimum the README "NATS Documentation Source" section should warn that the default expects the working directory to contain `assets/`.

### 5. `config.example.yaml` is internally inconsistent with the README and with the config types
- `config.example.yaml:13` still uses `docs_url:` — README diff renamed this to `docs_base_url:` and the viper loader (`internal/config/config.go:158`) only reads `docs_base_url`. So `docs_url` in the example is silently ignored.
- `config.example.yaml:35` uses `fetch_timeout: 30s` — but `FetchTimeout` is `int` seconds (`internal/config/config.go:24`) and `v.GetInt("fetch_timeout")` for `"30s"` returns 0, which then fails `Validate()` ("fetch_timeout must be positive").
- `config.example.yaml:49` says `max_search_results: 10` but the new default is 50 (and the README updated to 50).
- These are pre-existing, but you partially fixed them in the README diff in this same phase. Finish the job in `config.example.yaml` so it actually works when copied verbatim.

### 6. Parity acceptance not demonstrated against the criteria
- Phase 4 acceptance (`work/plans/implementation/phase-4-parity-and-cutover.md:111-115`) requires "≥95% canonical doc-ID overlap" and explained deltas.
- Your report says "site=186/archive=186 docs with expected path-set differences" but the actual overlap percentage and the list of expected-difference IDs aren't in the conversation. Please attach the parity report (or at least the `site-only IDs` / `archive-only IDs` lines + an explanation) so reviewers can confirm ≥95% before flipping the default.

## Smaller findings

- `.gitignore` lacks a trailing newline (`tail -c` shows `master/` then EOF). Cosmetic but noisy in some diff tools.
- `scripts/parity_check.go:82` sets `MaxConcurrent = 3` — only meaningful for `site` mode; harmless but noise.
- `scripts/parity_check.go:123-125` prints full ID lists with no head limit. With 186 docs and a non-trivial diff this output gets long. Truncate to first 20 with a `+N more` suffix.
- `internal/server/nats_archive_loader.go:171-176` calls the field `revision` but is fed `cfg.NATSArchiveBranch`. The naming asymmetry is minor but readers will wonder; a one-line comment on the struct field would help: *"revision label for SourceMetadata; supplied by NATSArchiveBranch"*.
- `internal/server/server.go:680-683` exposes `Server.NATSIndex()` purely for `scripts/parity_check.go`. That's fine, but the docstring should say "developer tooling only; not part of the MCP server surface" so it doesn't get used as a public API by accident.
- `internal/server/initialize_archive_test.go:220-246` — the smoke test is named "smoke" but it asserts both search and retrieve, which is good. Consider also asserting `srv.NATSIndex().Count() > 100` so a parser bug that produces a near-empty index can't pass with one accidental match.

## Summary

Phase 4 code is structurally fine — defaults flipped, loaders wired, tests added, README and example updated, parity tool exists. But the asset story is broken three ways (gitignored, not tracked; CI cannot fetch it; release tarballs don't include it), which means anyone except you running this branch from a clean checkout would hit a hard failure. Plan acceptance specifically called this out.

Required for **GO**:
1. Track `assets/nats.docs-master.zip` (remove the `.gitignore` exclusion and `git add -f`), or wire a deterministic fetch step into CI + goreleaser.
2. Confirm CI workflow runs end-to-end on a fresh checkout (`act` or a temp branch push).
3. Bundle the asset in goreleaser archives (and decide on the container image story).
4. Fix `config.example.yaml` (`docs_url`→`docs_base_url`, `30s`→`30`, `10`→`50`).
5. Attach the parity report so the ≥95% acceptance bar is auditable.

Once 1–4 are in and 5 is shared, this should be ready to merge.
