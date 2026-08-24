# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.1.0] - 2026-08-25

### Added
- `.gitignore`, `.editorconfig`, Dependabot config, issue/PR templates,
  `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, and a `.golangci.yml` lint config.
- Full CLI reference in the README (`run` flags and the `validate-paths`
  subcommand), plus a note about the differing default `cover-mode` between the
  CLI (`set`) and the Action (`atomic`).
- `golangci-lint` step in CI and a `make lint` target.
- Supply-chain hardening: a `govulncheck` CI job, and release-time cosign
  keyless signing, an SBOM (Syft, CycloneDX), and SLSA build provenance
  attestations for every archive and `checksums.txt`.
- Go Report Card workflow (`soulteary/goreportcard-action`) that grades the
  project locally, renders a self-contained SVG badge, and commits it back —
  no dependency on the (now sunset) `goreportcard.com` service.
- Patch-coverage gate (`scripts/check-patch-coverage.sh`, new lines `>= 90%`)
  wired into CI on pull requests, with a `--selftest` mode the CI verifies.
- `lefthook` pre-commit/pre-push config for local `gofmt`/`vet`/`lint`/`test`.
- `CODEOWNERS` and `.github/FUNDING.yml` governance files.
- Documented artifact verification (`cosign verify-blob` and
  `gh attestation verify`) in `SECURITY.md` and the README release section.

### Changed
- Renamed the CLI binary and install path from `gotestreport` to `gtr`
  (`go install .../cmd/gtr@latest`); all docs, scripts, and the Makefile now
  use `gtr`.
- Bumped the Go toolchain to 1.26.6 across `go.mod` and the test fixtures.
- Pinned all third-party GitHub Actions to full commit SHAs (with a trailing
  version comment) instead of floating major tags.
- Expanded `.golangci.yml` to an explicit allow-list that additionally enables
  `gosec`, `revive`, `gocritic`, `bodyclose`, and `nolintlint`, and pinned the
  linter version in CI.
- README coverage numbers are now derived from CI artifacts (single source of
  truth) rather than hand-maintained per-package percentages.

### Fixed
- Resolve a relative `-coverprofile` path against the process working
  directory rather than the `go test` directory, so coverage is collected
  correctly when `--directory` is set.

## [1.0.0]

### Added
- Initial release: single `go test -json` run producing a deterministic
  Markdown report, self-contained SVG coverage badge, and JSON output, with a
  GitHub Actions Job Summary, coverage gates, and optional in-repo write-back.

[Unreleased]: https://github.com/soulteary/go-test-report-action/compare/v1.1.0...HEAD
[1.1.0]: https://github.com/soulteary/go-test-report-action/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/soulteary/go-test-report-action/releases/tag/v1.0.0
