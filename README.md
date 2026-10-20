# ergo

CLI, check compilers, agent hooks and local MCP server that turn engineering decisions into enforced checks.

> Status: skeleton. The Go workspace, CLI and compilers land in a later change.

This is the Apache-2.0 adoption surface for ergo. One Go workspace will hold the EDR library, the check compilers, the connector SDK, the compiler plugin SDK, policy bundles, the CLI, agent hooks, the local MCP server, rules-file generators and the documentation site.

Module rule: `go.mod` in this repository requires `edr-spec` only.

## License

Apache License, Version 2.0. See [LICENSE](LICENSE).

Copyright 2026 Luca Bertelli.

## Contributing

Follow the contributor terms in [ergoprobat/.github](https://github.com/ergoprobat/.github/blob/main/CONTRIBUTING.md). Commits need a `Signed-off-by` trailer (`git commit -s`).
