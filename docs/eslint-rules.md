# ESLint rule file

`engine: eslint` reads one JSON file named by `ref` and writes `eslint.config.mjs`. Compile does not run Node.js. `target` is required and is `javascript` or `typescript`. Severity follows the check mode: `block` becomes `error`, and `warn` and `advise` become `warn`.

The generated config sets `noInlineConfig`, so an `eslint-disable` comment cannot turn a rule off. For `typescript` it imports `typescript-eslint` and sets `parserOptions.tsconfigRootDir` to the working directory.

## File

`files` is a list of glob patterns. `ignores` is optional. A pattern is not absolute, does not start with `!`, and does not contain `..`.

`rules` is an object. A rule id is a non-deprecated ESLint core rule. `typescript` also allows a non-deprecated `@typescript-eslint` rule. A value is `error` or `warn`, or an array whose first item is one of those and whose remaining items are the rule options. `off` is rejected. Compile replaces the severity from the check mode.

```json
{
  "files": ["services/**/*.ts"],
  "rules": {
    "@typescript-eslint/no-floating-promises": "error"
  }
}
```

## Evaluating with ipso

`ipso check eslint <path>... [--env name] [--report file]` runs every in-force `eslint` check. Each path is a file or directory in the repository, relative and slash-separated. ipso takes the verified `eslint.config.mjs`, reads it back into its rule data, compiles that data again, and refuses the check when the bytes differ. It then runs `eslint` from `PATH` from the repository root with that config and a suppressions file that holds `{}`. A repository `eslint.config.js`, an `eslint-suppressions.json`, and `eslint-disable` or `eslint` comments have no effect. Severity follows the check mode, and an in-force waiver turns a failure into a waiver. An error is never waived.

`eslint` must be `<dir>/node_modules/.bin/eslint` with `eslint` at 10.13.0 next to it, and for a `typescript` check `typescript-eslint` at 8.72.1 and `typescript` at 6.0.3 in the same `node_modules`. That directory must be outside the repository, for example from `npm ci --ignore-scripts` of a lockfile the runner owns, with `node_modules/.bin` on `PATH`. ipso writes each check's config below `<dir>/node_modules/.ipso-eslint-*` for the run, so the config imports `typescript-eslint` from that install, and removes it afterwards. A file that ESLint cannot parse, including a TypeScript file that no `tsconfig.json` includes, fails the check. So does a run that lints no file.

## Running ESLint by hand

Run from the repository root, with `typescript-eslint` installed where the config file can import it. The compiled check for decision `<decision-id>` and check index `<index>` is `.ipso/build/<decision-id>/<index>/eslint.config.mjs`. Pass `--suppressions-location` pointing at a file that holds `{}` so a repository `eslint-suppressions.json` is not applied. ESLint parses that file as JSON, so a zero-byte file fails.

```text
eslint --config .ipso/build/<decision-id>/<index>/eslint.config.mjs --suppressions-location <suppressions.json>
```
