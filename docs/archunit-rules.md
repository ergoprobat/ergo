# ArchUnit rule file

`engine: archunit` reads one JSON file named by `ref` and writes `ArchitectureTest.java`. Compile does not run a JVM. The check mode is stamped on the manifest and does not change the test. `target` is ignored the same way.

The generated class is `ergo.d<decision-id>.c<check-index>.ArchitectureTest`, with the decision id in lowercase. Every check can share `.ergo/build` as one test source root. The class imports the packages named in the file, skipping tests and jars, and runs one JUnit Jupiter test per rule.

## File

`packages` is a list of concrete package names, such as `com.acme.billing`. ArchUnit imports each package and its subpackages.

`rules` is a list. A rule name is kebab-case and unique in the file. Two kinds are accepted.

A `forbidden` rule has `source_packages` and `forbidden_packages`. A class in a source package must not depend on a class in a forbidden package.

A `layers` rule has `layers`, an ordered list of `{name, packages}` from top to bottom. A layer may be accessed only by the layers above it. Dependencies outside the layers are ignored. A layer name is kebab-case and unique in that rule.

A package pattern is identifiers joined by `.` or `..`, with an optional `..` at either end. `com.acme.billing.domain..` matches that package and its subpackages. The `packages` list itself does not use `..`.

```json
{
  "packages": ["com.acme.billing"],
  "rules": [
    {
      "name": "domain-not-to-adapters",
      "type": "forbidden",
      "source_packages": ["com.acme.billing.domain.."],
      "forbidden_packages": ["com.acme.billing.web..", "com.acme.billing.persistence.."]
    },
    {
      "name": "layers",
      "type": "layers",
      "layers": [
        {"name": "web", "packages": ["com.acme.billing.web.."]},
        {"name": "domain", "packages": ["com.acme.billing.domain.."]},
        {"name": "persistence", "packages": ["com.acme.billing.persistence.."]}
      ]
    }
  ]
}
```

## Running the test

Add `.ergo/build` as a test source root and put ArchUnit 1.5.1 and JUnit Jupiter on the test classpath. The generated test does not use the ArchUnit JUnit engine. From the repository root, the compiled check for decision `<decision-id>` and check index `<index>` is `.ergo/build/<decision-id>/<index>/ArchitectureTest.java`.
