# ergo

[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/ergoprobat/ergo/badge)](https://scorecard.dev/viewer/?uri=github.com/ergoprobat/ergo)
[![OpenSSF Best Practices](https://www.bestpractices.dev/projects/15758/badge)](https://www.bestpractices.dev/projects/15758)

CLI, check compilers, agent hooks and local MCP server that turn engineering decisions into enforced checks.

> Status: the CLI can initialize a repository, validate EDR files, and explain one decision. Compilers, hooks, and the MADR importer land in later changes.

This is the Apache-2.0 adoption surface for ergo. One Go workspace will hold the EDR library, the check compilers, the connector SDK, the compiler plugin SDK, policy bundles, the CLI, agent hooks, the local MCP server, rules-file generators and the documentation site.

Module rule: this module does not require `ergo-server`. The EDR JSON Schema is embedded from `edr-spec` so `ergo check` stays offline.

## Install

Pick one:

```bash
brew install ergoprobat/tap/ergo
```

```bash
go install github.com/ergoprobat/ergo/cmd/ergo@latest
```

Or download a release binary from [GitHub Releases](https://github.com/ergoprobat/ergo/releases). Each release publishes `checksums.txt`, keyless cosign signatures, and GitHub artifact attestations. The container image `ghcr.io/ergoprobat/ergo` is still private.

## Quick start

From a repository that will hold Engineering Decision Records (EDR files):

```bash
ergo init
ergo check
ergo explain <slug>
```

`ergo init` writes `.ergo/config.yaml` when that file is absent. The default glob is `docs/decisions/**/*.md`. Put EDR Markdown files under that path, then run `ergo check`. Use `ergo explain <slug>` to print the title, status, and Decision section for one file.

## Secure use

- `ergo check` and `ergo explain` run offline. They read local files that match the config globs and do not call a network service.
- The tool validates EDR structure against the embedded draft 0.1 schema. It does not execute decision text as code and does not run check engines yet.
- Prefer release binaries or Homebrew over building unreviewed forks when you need a trusted install. Verify release checksums and signatures when you download from GitHub Releases.
- Report security problems privately through [GitHub private vulnerability reporting](https://github.com/ergoprobat/ergo/security/advisories/new). Do not open a public issue for a vulnerability. See the [security policy](https://github.com/ergoprobat/.github/blob/main/SECURITY.md).

## Commands

- `ergo init` writes `.ergo/config.yaml` when that file is absent. The default glob is `docs/decisions/**/*.md`.
- `ergo check` validates every matched EDR file against the draft 0.1 schema and the required headings. It does not run check engines.
- `ergo explain <slug>` prints the title, status, and Decision section of that file.
- `ergo import` is reserved for the MADR and Nygard importer (ERGO-19). It does not write a file.

## CLI reference

Working directory is the repository root. There are no global flags beyond help and version.

| Invocation | Effect |
| --- | --- |
| `ergo init` | Create `.ergo/config.yaml` with `decisions: [docs/decisions/**/*.md]` when missing. Print `wrote .ergo/config.yaml` or `.ergo/config.yaml already exists`. Exit `0`, or `1` on I/O error, or `2` on bad usage. |
| `ergo check` | Load config (or the default globs), match EDR files, validate each. Print `<path>: ok`, `<path>: <problem>`, or `no EDR files found`. Exit `0` when every file is valid or none match, `1` when any file fails, `2` on bad usage. |
| `ergo explain <slug>` | Find the EDR whose front matter `slug` matches. Print `title:`, `status:`, then the `## Decision` body. Exit `0` on success, `1` when missing or ambiguous, `2` on bad usage. |
| `ergo import` | Not implemented yet. Prints that the importer is ERGO-19 and exits `2`. |
| `ergo help`, `ergo -h`, `ergo --help` | Print usage and exit `0`. |
| `ergo version`, `ergo -v`, `ergo --version` | Print `ergo <version> (<commit>, <date>)` and usage, then exit `0`. |

### Config

`.ergo/config.yaml`:

```yaml
decisions:
  - docs/decisions/**/*.md
```

`decisions` must list at least one glob. Paths are relative to the repository root. A missing config file uses the default above.

### Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Success |
| `1` | Runtime failure (I/O, validation failure, missing or ambiguous slug) |
| `2` | Usage error or unimplemented command |

## Release

A push to `main` opens a release-please pull request. Merging it tags `vX.Y.Z`. From v0.1.1, that tag is built by the pinned reusable workflow in `ergoprobat/.github`. The build publishes binaries, `checksums.txt`, a CycloneDX SBOM, keyless cosign signatures, and GitHub artifact attestations. The image is `ghcr.io/ergoprobat/ergo`. v0.1.1 is the first SLSA Build Level 3 release. v0.1.0 keeps its original bytes and has cosign signatures and a CycloneDX SBOM; it is not Level 3. The GHCR package is still private. The Homebrew formula is published to `ergoprobat/homebrew-tap` when `HOMEBREW_TAP_TOKEN` is set on this repository.

## License

Apache License, Version 2.0. See [LICENSE](LICENSE).

Copyright 2026 Luca Bertelli.

## Contributing

Follow the contributor terms in [ergoprobat/.github](https://github.com/ergoprobat/.github/blob/main/CONTRIBUTING.md). Commits need a `Signed-off-by` trailer (`git commit -s`).
