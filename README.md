# ergo

CLI, check compilers, agent hooks and local MCP server that turn engineering decisions into enforced checks.

> Status: the CLI can initialize a repository, validate EDR files, and explain one decision. Compilers, hooks, and the MADR importer land in later changes.

This is the Apache-2.0 adoption surface for ergo. One Go workspace will hold the EDR library, the check compilers, the connector SDK, the compiler plugin SDK, policy bundles, the CLI, agent hooks, the local MCP server, rules-file generators and the documentation site.

Module rule: this module does not require `ergo-server`. The EDR JSON Schema is embedded from `edr-spec` so `ergo check` stays offline.

## Commands

- `ergo init` writes `.ergo/config.yaml` when that file is absent. The default glob is `docs/decisions/**/*.md`.
- `ergo check` validates every matched EDR file against the draft 0.1 schema and the required headings. It does not run check engines.
- `ergo explain <slug>` prints the title, status, and Decision section of that file.
- `ergo import` is reserved for the MADR and Nygard importer (T-021). It does not write a file.

## Release

A push to `main` opens a release-please pull request. Merging it tags `vX.Y.Z`. From v0.1.1, that tag is built by the pinned reusable workflow in `ergoprobat/.github`. The build publishes binaries, `checksums.txt`, a CycloneDX SBOM, keyless cosign signatures, and GitHub artifact attestations. The image is `ghcr.io/ergoprobat/ergo`. v0.1.1 is the first SLSA Build Level 3 release. v0.1.0 keeps its original bytes and has cosign signatures and a CycloneDX SBOM; it is not Level 3. The GHCR package is still private. The Homebrew formula is published to `ergoprobat/homebrew-tap` when `HOMEBREW_TAP_TOKEN` is set on this repository.

## License

Apache License, Version 2.0. See [LICENSE](LICENSE).

Copyright 2026 Luca Bertelli.

## Contributing

Follow the contributor terms in [ergoprobat/.github](https://github.com/ergoprobat/.github/blob/main/CONTRIBUTING.md). Commits need a `Signed-off-by` trailer (`git commit -s`).
