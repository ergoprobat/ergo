# Semgrep templates

A check with `engine: semgrep` and `ref: ipso:<id>` compiles one of these templates. Severity follows the check mode. Each template is one Semgrep rule.

| id | title | languages |
| --- | --- | --- |
| `go/http-client-timeout` | Go HTTP clients set a timeout | go |
| `go/sql-no-string-query` | Go SQL queries use placeholders | go |
| `go/use-slog` | Go logging uses slog | go |
| `java/no-system-out` | Java logging does not use System.out | java |
| `java/sql-no-string-query` | Java SQL queries use placeholders | java |
| `javascript/no-eval` | JavaScript and TypeScript do not evaluate strings | javascript, typescript |
| `javascript/no-inner-html` | JavaScript and TypeScript do not assign innerHTML | javascript, typescript |
| `python/no-pickle` | Python deserialization does not use pickle | python |
| `python/no-shell-true` | Application code must not call a shell with a string | python |
| `python/requests-timeout` | Python HTTP requests set a timeout | python |
