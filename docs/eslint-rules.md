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

## Running ESLint

Run from the repository root, with `typescript-eslint` installed where the config file can import it. The compiled check for decision `<decision-id>` and check index `<index>` is `.ipso/build/<decision-id>/<index>/eslint.config.mjs`. Pass `--suppressions-location` pointing at an empty file so a repository `eslint-suppressions.json` is not applied.

```text
eslint --config .ipso/build/<decision-id>/<index>/eslint.config.mjs --suppressions-location <empty.json>
```
