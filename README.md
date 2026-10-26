# ergo

[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/ergoprobat/ergo/badge)](https://scorecard.dev/viewer/?uri=github.com/ergoprobat/ergo)
[![OpenSSF Best Practices](https://www.bestpractices.dev/projects/15758/badge)](https://www.bestpractices.dev/projects/15758)

CLI, check compilers, agent hooks and local MCP server that turn engineering decisions into enforced checks.

> Status: the CLI can initialize a repository, validate EDR files, explain one decision, import MADR or Nygard ADRs, compile in-force checks, and evaluate a Terraform or OpenTofu plan. The OPA compiler is registered. A check with `ref: ergo:<provider>/<name>` and `target: terraform-plan` compiles a built-in template. Repository Rego is one `.rego` file. The template list is [docs/templates.md](docs/templates.md). The dependency-cruiser compiler is registered. A check with `engine: depcruise` reads one JSON rule file and writes `depcruise.json`. It does not run dependency-cruiser. The Semgrep compiler is registered. A check with `engine: semgrep` reads one YAML rule file, or `ref: ergo:<language>/<name>` for a built-in template, and writes `semgrep.yaml`. It does not run Semgrep. The template list is [docs/semgrep-templates.md](docs/semgrep-templates.md). `ergo check plan` evaluates the compiled checks against `terraform show -json` or `tofu show -json` output. The CI-binding compiler is registered. A check with `engine: ci-binding` writes `binding.json` from `job`. `ergo check ci` reads an evidence file for one commit and fails when that job is missing or did not succeed. The Checkov compiler is registered. A check with `engine: checkov` writes `binding.json` from a check id in `ref`. `ergo check checkov` reads a Checkov JSON report and fails when that id failed, was skipped, or was not reported. Neither command calls a network service. The `ci / conformance` job compiles the OPA, Semgrep, and dependency-cruiser fixtures and fails the release when a pinned engine disagrees with the expected verdict. Hooks land in later changes.

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

- `ergo check`, `ergo explain`, `ergo compile`, `ergo check plan`, `ergo check ci`, and `ergo check checkov` run offline. They read local files that match the config globs and do not call a network service.
- The tool validates EDR structure against the embedded draft 0.1 schema. It does not execute decision text as code. `ergo compile` writes artifacts for registered engines. A dependency-cruiser rule file is JSON. Compile validates it and does not execute it. Options that load code, write a cache, or grandfather violations are rejected. `ergo check plan` evaluates the compiled Rego against a plan and writes a verdict report. The report carries the plan hash and verdict counts. Resource addresses and messages stay on the terminal. `ergo check ci` and `ergo check checkov` evaluate compiled bindings against a local evidence file. The report carries hashes and verdict counts. Job names, Checkov resource names, and file paths stay on the terminal.
- Prefer release binaries or Homebrew over building unreviewed forks when you need a trusted install. Verify release checksums and signatures when you download from GitHub Releases.
- Report security problems privately through [GitHub private vulnerability reporting](https://github.com/ergoprobat/ergo/security/advisories/new). Do not open a public issue for a vulnerability. See the [security policy](https://github.com/ergoprobat/.github/blob/main/SECURITY.md).

## Commands

- `ergo init` writes `.ergo/config.yaml` when that file is absent. The default glob is `docs/decisions/**/*.md`.
- `ergo check` validates every matched EDR file against the draft 0.1 schema and the required headings. It does not run check engines.
- `ergo explain <slug>` prints the title, status, and Decision section of that file.
- `ergo import` rewrites MADR and Nygard ADR files under `docs/adr` and `docs/decisions` into EDR files. Fields the source did not contain are filled in, and `inferred` is set to true. A file that is already a valid EDR file is left unchanged.
- `ergo check plan <plan.json> [--env name] [--report file]` evaluates the compiled checks in `.ergo/build/` against one plan. Run `ergo compile` first, and again after a decision changes: the command trusts the build for status, waivers, and environments, and re-checks only the dates. Without `--env`, checks that name an environment are evaluated too. The plan is the JSON from `terraform show -json` or `tofu show -json`, or `-` to read it from stdin. A check is evaluated when its engine is `opa`, its target is `terraform-plan`, its decision is in force today, and `--env` is absent or the decision applies in that environment. A waiver in force today turns the violations of its decision into a waived result. The report written by `--report` contains the plan hash, the build hash, and one verdict per evaluated check. It does not contain plan values, resource addresses, or messages.
- `ergo compile` compiles checks on records that are in force today. A record is in force when its status is `accepted`, `verified`, or `needs-review` and today falls in `[valid_from, valid_to)`. It replaces `.ergo/build/` with a manifest and the artifact files. The directory contains a `.gitignore` of `*`. A check with no registered compiler is reported and skipped. `llm-advisory` is reported as advisory and is not compiled. An explicit check `mode` is kept. Otherwise `blocking`, `warning`, and `advisory` become `block`, `warn`, and `advise`. For `engine: opa`, `ref: ergo:<provider>/<name>` selects a built-in Terraform plan template and requires `target: terraform-plan`. Any other `ref` is one Rego file in the repository. The compiler writes `policy.rego` after the module parses, and `terraform.rego` for a template. It does not evaluate the plan. For `engine: depcruise`, `ref` is one JSON rule file in the repository. The compiler writes `depcruise.json` and does not run dependency-cruiser. Rule severity follows the check mode (`block` to `error`, `warn` stays `warn`, `advise` to `info`), and a rule set to `ignore` stays `ignore`. `extends` is rejected. So are options that load code, select another compiler, follow symlinks, or write files (`webpackConfig`, `babelConfig`, `cache`, `externalModuleResolutionStrategy`, `parser`, `detectJSDocImports`, `detectImportsInAmbientModules`, `preserveSymlinks`), options that grandfather violations (`knownViolations`, `baseline`, `ignoreKnown`), and options that only shape a report (`focus`, `reaches`, `highlight`, `collapse`, `affected`, `prefix`, `suffix`, `progress`, `reporterOptions`). `baseDir` and `tsConfig.fileName` must stay inside the repository. `doNotFollow`, `exclude`, and `includeOnly` are regular expressions, not paths. Run the result from the repository root with `depcruise --config .ergo/build/<decision-id>/<check-index>/depcruise.json <paths>`. For `engine: semgrep`, `ref` is one `.yaml` or `.yml` rule file, or `ergo:<language>/<name>` for a built-in template ([docs/semgrep-templates.md](docs/semgrep-templates.md)). The compiler writes `semgrep.yaml` and does not run Semgrep. Severity follows the check mode (`block` to `ERROR`, `warn` to `WARNING`, `advise` to `INFO`). Autofix, join mode, extract mode, and version gates are rejected. Run the result with `semgrep scan --config .ergo/build/<decision-id>/<check-index>/semgrep.yaml --metrics=off`. For `engine: ci-binding`, `job` is the name of an existing CI job. The compiler writes `binding.json` and does not call the CI provider. `ergo check ci <evidence.json> --sha <sha>` fails when that job is missing from the evidence file or did not conclude `success`. For `engine: checkov`, `ref` is a Checkov id such as `CKV_AWS_24`. The compiler writes `binding.json` and does not run Checkov. `ergo check checkov <results.json>` fails when that id is failed, skipped, or absent. Do not pass `--quiet` to Checkov: that JSON drops passed checks, so a check that passed is reported as not reported.

## CI job evidence

`ergo check ci` does not call GitHub. It trusts the evidence file and `--sha`. The workflow below is what keeps another app from spoofing a job name: it asks only for check runs from the GitHub Actions app, `app_id` 15368. `$SHA` is the pull request head, which is the commit the jobs ran on. The job needs `checks: read`.

```yaml
permissions:
  checks: read
  contents: read
steps:
  - uses: actions/checkout@v4
  - name: Job evidence
    env:
      SHA: ${{ github.event.pull_request.head.sha || github.sha }}
      GH_TOKEN: ${{ github.token }}
    run: |
      gh api --method GET -H "Accept: application/vnd.github+json" \
        "repos/${{ github.repository }}/commits/${SHA}/check-runs?filter=latest&app_id=15368&per_page=100" \
        --jq '{evidence_version:1, sha:env.SHA, jobs:[.check_runs[] | {name:.name, conclusion:.conclusion}]}' \
        > evidence.json
      ergo check ci evidence.json --sha "$SHA"
```

`gh` reads `GH_TOKEN`. `env.SHA` is how `gh --jq` reads the environment. A job that has not finished has `conclusion: null`, and the verdict fails closed. ergo trusts the evidence file and `--sha`. Write the file in this job. The SHA is `github.event.pull_request.head.sha` or `github.sha`. This snippet reads one page of 100 check runs. A repository with more must paginate. A job that is not in the file fails closed.

## CLI reference

Working directory is the repository root. There are no global flags beyond help and version.

| Invocation | Effect |
| --- | --- |
| `ergo init` | Create `.ergo/config.yaml` with `decisions: [docs/decisions/**/*.md]` when missing. Print `wrote .ergo/config.yaml` or `.ergo/config.yaml already exists`. Exit `0`, or `1` on I/O error, or `2` on bad usage. |
| `ergo check` | Load config (or the default globs), match EDR files, validate each. Print `<path>: ok`, `<path>: <problem>`, or `no EDR files found`. Exit `0` when every file is valid or none match, `1` when any file fails, `2` on bad usage. |
| `ergo explain <slug>` | Find the EDR whose front matter `slug` matches. Print `title:`, `status:`, then the `## Decision` body. Exit `0` on success, `1` when missing or ambiguous, `2` on bad usage. |
| `ergo import [path...]` | Rewrite each MADR or Nygard Markdown file in place as an EDR file. With no path, use `docs/adr` and `docs/decisions`. Print `<path>: imported inferred`, `<path>: skipped` for a file that already validates, `<path>: <problem>`, or `no ADR files found`. Exit `0` when every file is imported, skipped, or none are found, `1` when any file fails, `2` on bad usage. |
| `ergo check plan <plan.json\|-> [--env name] [--report file]` | Evaluate compiled `terraform-plan` checks in `.ergo/build/` against one plan. Print `<slug>: checks[N] (mode): pass`, `<slug>: checks[N] (mode): waived`, `<slug>: checks[N] (mode): <address>: <message>`, or `<slug>: checks[N] (mode): error: <reason>`. Exit `0` when no block-mode check has a violation and no check errors, `1` when one does, when the plan is not a plan, or when `.ergo/build` is missing or its artifacts do not match the manifest, `2` on bad usage. |
| `ergo check ci <evidence.json\|-> --sha <sha> [--env name] [--report file]` | Evaluate compiled `ci-binding` checks against a job evidence file for that commit. Exit `0` when every selected block-mode job succeeded, `1` when a bound job is missing or did not succeed, when the evidence is for another sha, or when `.ergo/build` is missing, `2` on bad usage. |
| `ergo check checkov <results.json\|-> [--env name] [--report file]` | Evaluate compiled `checkov` checks against a Checkov JSON report. Exit `0` when every selected block-mode id passed, `1` when an id failed, was skipped, or was not reported, or when the report has a parsing error, `2` on bad usage. |
| `ergo compile` | Compile checks on in-force records into `.ergo/build/`. An OPA check with `ref: ergo:<provider>/<name>` and `target: terraform-plan` compiles a built-in template ([docs/templates.md](docs/templates.md)). A depcruise check writes `depcruise.json` from a JSON rule file. A Semgrep check writes `semgrep.yaml` from a YAML rule file or a built-in template ([docs/semgrep-templates.md](docs/semgrep-templates.md)). A ci-binding check writes `binding.json` from `job`. A Checkov check writes `binding.json` from `ref`. Print `<path>: checks[N] (engine): compiled K artifacts`, `<path>: checks[N] (engine): no compiler`, `<path>: checks[N] (engine): advisory, not compiled`, `<path>: skipped, not in force`, or `no EDR files found`. Exit `0` when every in-force check compiles or has no compiler, `1` when a record is invalid, a compile fails, two records share an id, or `.ergo/build` is a symlink, `2` on bad usage. A failure leaves the previous build in place. |
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
| `2` | Usage error |

## Release

A push to `main` opens a release-please pull request. Merging it tags `vX.Y.Z`. From v0.1.1, that tag is built by the pinned reusable workflow in `ergoprobat/.github`. The build publishes binaries, `checksums.txt`, a CycloneDX SBOM, keyless cosign signatures, and GitHub artifact attestations. The image is `ghcr.io/ergoprobat/ergo`. v0.1.1 is the first SLSA Build Level 3 release. v0.1.0 keeps its original bytes and has cosign signatures and a CycloneDX SBOM; it is not Level 3. The GHCR package is still private. The Homebrew formula is published to `ergoprobat/homebrew-tap` when `HOMEBREW_TAP_TOKEN` is set on this repository.

## License

Apache License, Version 2.0. See [LICENSE](LICENSE).

Copyright 2026 Luca Bertelli.

## Contributing

Follow the contributor terms in [ergoprobat/.github](https://github.com/ergoprobat/.github/blob/main/CONTRIBUTING.md). Commits need a `Signed-off-by` trailer (`git commit -s`).
