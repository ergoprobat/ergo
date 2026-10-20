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

## License

Apache License, Version 2.0. See [LICENSE](LICENSE).

Copyright 2026 Luca Bertelli.

## Contributing

Follow the contributor terms in [ergoprobat/.github](https://github.com/ergoprobat/.github/blob/main/CONTRIBUTING.md). Commits need a `Signed-off-by` trailer (`git commit -s`).
