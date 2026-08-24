# Security Policy

## Supported versions

Security fixes are applied to the latest `v1` release. Pin the action to a full
commit SHA in production and update it via Dependabot or Renovate.

## Reporting a vulnerability

Please report suspected vulnerabilities privately via GitHub Security Advisories
("Report a vulnerability" on the repository's Security tab) rather than opening a
public issue. Include reproduction steps and the affected version or commit.

## Security model

This action is designed to run untrusted code (a repository's own tests) with a
minimal, auditable trust boundary:

- **No `pull_request_target` for PR code.** PR test code must be run under the
  `pull_request` event with a read-only token. This action never requires
  `pull_request_target`.
- **Fork PRs are read-only.** Fork pull requests use a read-only `GITHUB_TOKEN`
  and require no secrets. Setting `commit: true` on a `pull_request` event is
  refused with a warning; write-back only happens on default-branch `push` or a
  maintainer-triggered `workflow_dispatch`.
- **Least privilege.** PR workflows use `contents: read`. Only the report
  write-back workflow uses `contents: write`.
- **No shell injection.** All user inputs are passed as explicit process
  arguments. Nothing is routed through `eval`, `bash -c`, or unquoted shell
  expansion. Extra `go test` arguments are tokenized by the Go CLI, not a shell.
- **Path containment.** The project directory and every output path are
  validated by the Go CLI (`validate-paths`) to stay inside `GITHUB_WORKSPACE`.
  Validation resolves already-existing parent directories through
  `EvalSymlinks`, so a symlinked parent cannot redirect output outside the
  workspace.
- **Release integrity.** Prebuilt binaries are verified against `checksums.txt`
  (SHA256) before use; a failed download or checksum falls back to a source
  build from the pinned action checkout. Each release archive, `checksums.txt`,
  and the SBOM are additionally signed with [cosign](https://docs.sigstore.dev/)
  keyless signing (Sigstore transparency log, no long-lived keys), ship a
  CycloneDX SBOM, and carry an SLSA build-provenance attestation. Verify an
  archive with:

  ```bash
  cosign verify-blob \
    --certificate gtr_<version>_<os>_<arch>.tar.gz.pem \
    --signature  gtr_<version>_<os>_<arch>.tar.gz.sig \
    --certificate-identity-regexp 'https://github.com/soulteary/go-test-report-action/.+' \
    --certificate-oidc-issuer https://token.actions.githubusercontent.com \
    gtr_<version>_<os>_<arch>.tar.gz

  gh attestation verify gtr_<version>_<os>_<arch>.tar.gz \
    --repo soulteary/go-test-report-action
  ```
- **Escaping.** Test names, package names, and error text are escaped for
  Markdown, GitHub workflow commands, and SVG/XML output.
- **Scoped Git writes.** Write-back only ever `git add`s the three stable report
  files. It never uses `git add .` / `git add -A` and never auto-rebases; a
  non-fast-forward push fails loudly.
- **No secret leakage.** Tokens, environment secrets, and full event payloads
  are never printed. Failure stacks are truncated in the Job Summary; full raw
  output is kept only in artifacts.
