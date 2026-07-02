# rvs CLI — Release Notes

## v0.3.0 (unreleased — prepared, not yet tagged/published)

Changes since v0.2.0 (`496faf8`):

### Added
- `rvs effort summary` — read-side CLI command to summarize effort log entries (CLI-03). Adds `GetEffortSummary` to the API client.

### Fixed
- Bumped `go.mod` to `go 1.24` to match the `t.Context()` API used by the new effort-summary tests (was declared `go 1.23`, which made `go vet` fail against the installed 1.24+ toolchain).

### Docs
- Documented `RVS_TOKEN`, token path, shell completions, and a full commands table in the README.

---

## Publishing checklist (when GitHub Actions billing is restored)

This release was validated and built **locally only** — GitHub Actions billing is currently locked, so no CI ran on GitHub and nothing was published to GitHub Releases.

Local validation performed on `main` @ `1be8c02`:
- `go build ./...` — exit 0
- `go vet ./...` — exit 0
- `go test ./...` — all packages pass

Local snapshot binaries built with `goreleaser release --snapshot --clean --skip=publish` (darwin/linux × amd64/arm64), output in `dist/` (gitignored, not committed):
- `dist/rvs_darwin_amd64.tar.gz`
- `dist/rvs_darwin_arm64.tar.gz`
- `dist/rvs_linux_amd64.tar.gz`
- `dist/rvs_linux_arm64.tar.gz`
- `dist/checksums.txt`

To actually cut and publish v0.3.0 once billing is back:

```sh
git tag v0.3.0
git push origin v0.3.0
goreleaser release --clean
```
