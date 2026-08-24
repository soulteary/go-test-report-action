# Contributing

Thanks for your interest in improving **go-test-report-action**.

## Development setup

Requires Go (see [`go.mod`](go.mod) for the version) and `make`.

```bash
git clone https://github.com/soulteary/go-test-report-action
cd go-test-report-action
make all        # fmt-check + vet + test
```

## Common tasks

| Command | What it does |
| --- | --- |
| `make build` | Build the `gtr` binary into `bin/`. |
| `make test` | Run all unit and integration tests. |
| `make cover` | Run tests with coverage and print the total. |
| `make cover-html` | Open the HTML coverage report. |
| `make fmt` | Apply `gofmt` to all tracked Go files. |
| `make fmt-check` | Fail if any file needs formatting. |
| `make vet` | Run `go vet ./...`. |
| `make lint` | Run `golangci-lint` (install it first, see below). |
| `make patch-cover` | Enforce patch coverage (new lines `>= 90%`) locally. |
| `make smoke` | Build and run the CLI against the passing fixture. |

Install `golangci-lint` locally with:

```bash
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

Optionally install [`lefthook`](https://github.com/evilmartians/lefthook) to run
`gofmt`/`vet`/`lint` on commit and the tests on push:

```bash
go install github.com/evilmartians/lefthook@latest
lefthook install
```

## Pull requests

- Keep the three stable report artifacts deterministic — no timestamps, elapsed
  time, run IDs, or absolute paths in `report`/`badge`/`json` output.
- Add or update tests for behavior changes. Core packages carry high coverage;
  CI enforces project coverage `>= 85%`.
- Run `make all` and `make lint` before pushing.
- Update the [`CHANGELOG.md`](CHANGELOG.md) `Unreleased` section for user-facing
  changes.
- Keep [`README.md`](README.md), [`action.yml`](action.yml), and the CLI flags
  in sync when you add or change inputs/outputs.

## Reporting security issues

Please follow [`SECURITY.md`](SECURITY.md) rather than opening a public issue.

## Code of conduct

This project adheres to the [Contributor Covenant](CODE_OF_CONDUCT.md).
