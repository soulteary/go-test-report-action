# Go Test Report Action

Run your Go tests once, then generate a deterministic Markdown report, a
self-contained SVG coverage badge, and machine-readable JSON — plus a rich
GitHub Actions Job Summary — with optional coverage gates and in-repo
write-back. No Codecov, Coveralls, or Shields.io: every artifact is produced
locally from your own `go test` run, so it works on public repos, GitHub
Enterprise, air-gapped mirrors, and projects that will not upload source or
coverage data.

[![Go Test Coverage](./.github/coverage.svg)](./.github/go-test-report.md)
[![CI](https://github.com/soulteary/go-test-report-action/actions/workflows/ci.yml/badge.svg)](https://github.com/soulteary/go-test-report-action/actions/workflows/ci.yml)
[![CodeQL](https://github.com/soulteary/go-test-report-action/actions/workflows/codeql.yml/badge.svg)](https://github.com/soulteary/go-test-report-action/actions/workflows/codeql.yml)
[![Release](https://img.shields.io/github/v/release/soulteary/go-test-report-action?sort=semver)](https://github.com/soulteary/go-test-report-action/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/soulteary/go-test-report-action)](https://goreportcard.com/report/github.com/soulteary/go-test-report-action)
[![License](https://img.shields.io/github/license/soulteary/go-test-report-action)](LICENSE)

## What it does

- Runs `go test -json -count=1 -covermode=<mode> -coverprofile=...` a single
  time and derives both test statistics and coverage from that one run.
- Parses structured `-json` events (not terminal text) for pass/fail/skip/panic
  and compile failures; counts same-named tests in different packages
  independently and aggregates subtests under their parent.
- Parses the native Go cover profile with `golang.org/x/tools/cover` (not regex
  over `go tool cover` output); packages with zero executable statements are
  reported as `N/A` rather than 0%.
- Emits three deterministic files (no timestamps, elapsed time, run IDs, or
  absolute paths) so committing them back produces no noisy diffs.
- Enforces total-coverage and per-package coverage thresholds, failing the job
  only after reports and outputs are produced.

## Quick start (30 seconds)

```yaml
- uses: actions/checkout@v7
- id: test
  uses: soulteary/go-test-report-action@v1
  with:
    coverage_threshold: "80"
```

Outputs land in `.github/go-test-report.md`, `.github/coverage.svg`, and
`.github/go-test-report.json`; raw `test.jsonl` and `coverage.out` go to
`.github/test-results/` for artifact upload.

## Pull request workflow (read-only)

PR checks run tests and gates with `contents: read` and no secrets. Fork PRs use
a read-only token. See [examples/pull-request.yml](examples/pull-request.yml).

```yaml
permissions:
  contents: read
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
      - uses: soulteary/go-test-report-action@v1
        with:
          race: "true"
          coverage_threshold: "80"
          package_threshold: "60"
          commit: "false"
      - if: always()
        uses: actions/upload-artifact@v7
        with:
          name: go-test-report-${{ github.run_id }}
          path: .github/test-results/
```

## Default-branch workflow (write-back)

Only this workflow needs `contents: write`. It regenerates the report and
commits the three stable files when they change. See
[examples/update-report.yml](examples/update-report.yml).

```yaml
permissions:
  contents: write
concurrency:
  group: go-test-report-${{ github.ref }}
  cancel-in-progress: true
jobs:
  report:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
      - uses: soulteary/go-test-report-action@v1
        with:
          coverage_threshold: "80"
          commit: "true"
```

In your README, reference the generated badge with a relative link:

```markdown
[![Go Test Coverage](./.github/coverage.svg)](./.github/go-test-report.md)
```

## Inputs

| Input | Default | Description |
| --- | --- | --- |
| `directory` | `.` | Go project root, relative to `GITHUB_WORKSPACE`. |
| `go_version_file` | `go.mod` | Go version file for `actions/setup-go`. |
| `packages` | `./...` | Package patterns to test. |
| `exclude` | (empty) | Regexps matched against import path, one per line. |
| `race` | `false` | Enable the race detector (requires `cover_mode=atomic`). |
| `cover_mode` | `atomic` | `set`, `count`, or `atomic`. |
| `cover_pkg` | (empty) | Value for `go test -coverpkg`. |
| `timeout` | `10m` | `go test -timeout`. |
| `test_args` | (empty) | Extra args, safely tokenized (never shell-evaluated). |
| `coverage_threshold` | `0` | Total coverage gate; `0` disables. |
| `package_threshold` | `0` | Per-package coverage gate; `0` disables. |
| `report_output` | `.github/go-test-report.md` | Stable Markdown report. |
| `badge_output` | `.github/coverage.svg` | Self-contained SVG badge. |
| `json_output` | `.github/go-test-report.json` | Stable JSON report. |
| `raw_output` | `.github/test-results` | Raw results dir (for artifacts). |
| `commit` | `false` | Commit the stable reports back. |
| `commit_on_failure` | `false` | Commit even when tests fail. |
| `commit_message` | `chore: update Go test report [skip ci]` | Commit message. |
| `version` | `latest` | CLI release version, or `source` to build from the action checkout. |

## Outputs

| Output | Description |
| --- | --- |
| `status` | `passed`, `test_failed`, `coverage_failed`, or `error`. |
| `coverage` | Total coverage percentage, e.g. `84.10`. |
| `tests` / `passed` / `failed` / `skipped` | Test counts. |
| `report` / `badge` / `json` | Paths to the generated files. |
| `coverage_profile` | Path to the raw cover profile. |
| `exit_code` | Semantic CLI exit code (see below). |
| `committed` | `true` if a report commit was produced. |
| `commit_sha` | SHA of the produced commit, or empty. |

## Local CLI usage

The action is a thin wrapper around a Go CLI you can run locally to reproduce CI:

```bash
go install github.com/soulteary/go-test-report-action/cmd/gtr@latest

gtr run \
  --packages ./... \
  --cover-mode atomic \
  --coverage-threshold 80 \
  --json-output test-report.json \
  --markdown-output test-report.md \
  --svg-output coverage.svg
```

> **Note:** the CLI's `run` default `--cover-mode` is `set`, whereas the Action
> layer defaults `cover_mode` to `atomic`. Pass `--cover-mode atomic` locally to
> reproduce CI exactly (required if you also pass `--race`).

### Subcommands

| Subcommand | Description |
| --- | --- |
| `run` | Run tests, build the report/badge/JSON, and exit with a semantic code. |
| `validate-paths` | Resolve and verify that paths stay inside a workspace root; used internally by the composite Action. Usage: `gtr validate-paths -workspace <root> [-path p ...]`. |
| `help`, `-h`, `--help` | Print usage. |

### `run` flags

| Flag | Default | Description |
| --- | --- | --- |
| `--directory` | `.` | Module root directory to run tests in. |
| `--packages` | `./...` | Package patterns to test (space-separated). |
| `--exclude` | (empty) | Regexp matched against import path to exclude; repeatable. |
| `--race` | `false` | Enable the race detector (requires `--cover-mode atomic`). |
| `--cover-mode` | `set` | Coverage mode: `set`, `count`, or `atomic`. |
| `--cover-pkg` | (empty) | Value for `go test -coverpkg`. |
| `--timeout` | `10m` | `go test -timeout` value. |
| `--test-args` | (empty) | Extra shell-like args appended to `go test` (tokenized, never shell-evaluated). |
| `--coverage-threshold` | `0` | Minimum total coverage percentage `[0,100]`; `0` disables. |
| `--package-threshold` | `0` | Minimum per-package coverage percentage `[0,100]`; `0` disables. |
| `--json-output` | `test-report.json` | Path for the deterministic JSON report. |
| `--markdown-output` | `test-report.md` | Path for the deterministic Markdown report. |
| `--svg-output` | `coverage.svg` | Path for the coverage SVG badge. |
| `--summary-output` | (stdout) | Path for the dynamic Job Summary (empty writes to stdout). |
| `--raw-output-dir` | (temp dir) | Directory for raw artifacts (`test.jsonl`, `coverage.out`); empty uses a temp dir. |
| `--max-failures` | (see CLI) | Max failing cases rendered in Markdown. |
| `--max-packages` | (see CLI) | Max package rows rendered in Markdown. |
| `--sha` | (empty) | Commit SHA (Job Summary only). |
| `--branch` | (empty) | Branch name (Job Summary only). |
| `--runner` | (empty) | Runner label (Job Summary only). |

### Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Tests passed and coverage satisfied. |
| `10` | Test or compile failure. |
| `11` | Total coverage below threshold. |
| `12` | A package below the per-package threshold. |
| `20` | Input/path/configuration error. |
| `21` | Go toolchain or internal execution error. |

## Coverage and no-test packages (precise rules)

- Total coverage = executed statements / total statements across included
  packages, using raw statement counts for gate comparisons and displaying two
  decimals.
- A package with **zero executable statements** is `N/A` and never fails the
  per-package gate.
- A package that has statements but **no tests** counts its statements as
  uncovered (0%); it will fail a total or per-package coverage gate if one is
  set. This is intentional — the code exists but is untested.
- Packages removed by `exclude` do not appear in the report or gates.

## Monorepo / multiple modules

Run the action once per module using `directory` and distinct output paths:

```yaml
- uses: soulteary/go-test-report-action@v1
  with:
    directory: services/api
    report_output: services/api/.github/go-test-report.md
    badge_output: services/api/.github/coverage.svg
    json_output: services/api/.github/go-test-report.json
- uses: soulteary/go-test-report-action@v1
  with:
    directory: services/worker
    report_output: services/worker/.github/go-test-report.md
    badge_output: services/worker/.github/coverage.svg
    json_output: services/worker/.github/go-test-report.json
```

Automatic Go workspace discovery and baseline coverage-drop comparison are
planned for a later version.

## FAQ / troubleshooting

- **Report is not committed.** Write-back requires all of: `commit: true`, a
  default-branch `push` or maintainer `workflow_dispatch`, not a fork PR, tests
  passing (or `commit_on_failure: true`), and at least one of the three stable
  files actually changing. Setting `commit: true` on a `pull_request` event is
  refused with a warning.
- **`Permission denied` / no write access.** The write-back workflow needs
  `permissions: contents: write`; PR workflows are read-only by design.
- **`race` fails or CGO errors.** `-race` requires `cover_mode: atomic` and a C
  toolchain on the runner; the action returns a config error (exit 20) if
  `race` is set with a non-atomic cover mode.
- **Private module dependencies.** Configure `GOPRIVATE`/`GONOSUMCHECK` and any
  auth in a prior step; this action does not fetch code itself beyond your
  module's normal `go test`.

## Release and versioning

- Semantic versioning; publishing `v1.0.0` also updates the moving `v1` tag.
- Release assets are named `gtr_<version>_<os>_<arch>.tar.gz` (`.zip`
  on Windows) for Linux/macOS/Windows on amd64/arm64, with a `checksums.txt`.
- Every archive, `checksums.txt`, and the CycloneDX SBOM are signed with cosign
  keyless signing (`.sig` + `.pem` alongside each file) and archives carry an
  SLSA build-provenance attestation. See [SECURITY.md](SECURITY.md) for the
  `cosign verify-blob` / `gh attestation verify` commands.
- `version: latest` resolves the newest release; a fixed version downloads that
  release. On download or checksum failure the action falls back to building
  from its own checkout.
- Pin to a full commit SHA in production and update via Dependabot/Renovate.

## Quality

Verified on Ubuntu, macOS, and Windows runners on every push and pull request.
The project coverage gate is enforced in CI (`>= 85%` total); the live coverage
percentage is the badge above, generated by the action running on itself — that
badge and the [`.github/go-test-report.md`](./.github/go-test-report.md) report
are the single source of truth rather than any number hand-copied here. Patch
coverage on pull requests is gated separately (new lines `>= 90%`). See
[SECURITY.md](SECURITY.md) for the trust boundary.

## License

Apache-2.0. See [LICENSE](LICENSE).
