# ArchUnit rule file

`engine: archunit` reads one JSON file named by `ref` and writes `ArchitectureTest.java`. Compile does not run a JVM. The check mode is stamped on the manifest and does not change the test. `target` is ignored the same way.

The generated class is `ipso.d<decision-id>.c<check-index>.ArchitectureTest`, with the decision id in lowercase. Every check can share `.ipso/build` as one test source root. The class imports the packages named in the file, skipping tests and jars, and runs one JUnit Jupiter test per rule.

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

Add `.ipso/build` as a test source root and put ArchUnit 1.5.1 and JUnit Jupiter on the test classpath. The generated test does not use the ArchUnit JUnit engine. From the repository root, the compiled check for decision `<decision-id>` and check index `<index>` is `.ipso/build/<decision-id>/<index>/ArchitectureTest.java`.

## Evaluate with ipso

`ipso check archunit <classes-dir>... --jars <dir> [--env name] [--report file]` runs the compiled test for every in-force `archunit` check. Run `ipso compile` first, then build the project so the class directories exist.

`<classes-dir>` is a compiled class directory relative to the repository root, such as `target/classes` or `build/classes/java/main`. ipso copies only the `.class` files below it. Resource files next to the classes, such as `archunit.properties` or `archunit_ignore_patterns.txt`, are not read. The directory layout is kept, so the `DO_NOT_INCLUDE_TESTS` option that the test sets matches the same locations as in a native run. ipso filters nothing itself, and a named `target/test-classes` or `build/classes/java/test` imports as nothing and fails as `archunit-empty`. A symlink anywhere below a named directory fails the check. Naming `.` copies every `.class` file in the repository and keeps its path from the root, so ArchUnit does not find the packages and the check fails as `archunit-empty`. Name the class output directory.

`--jars` is a directory that holds the three jars in the table. ipso checks each sha256 and runs a copy of the verified bytes. The `install.sh` next to `jars.lock` under `internal/compiler/archunit/testdata/tool` downloads them from Maven Central into `lib/`.

| Jar | sha256 |
| --- | --- |
| `archunit-1.5.1.jar` | `a4dbfc51c90005ad6ac9967672a7efb43aa6928287a57238b28e60e0a75c5c6f` |
| `slf4j-api-2.0.19.jar` | `e91ff6d720609e7a194ffe758c3ed5c84e798617ae07b0a0f6a4fe229741b4bb` |
| `junit-platform-console-standalone-6.1.3.jar` | `e62b96ac475dbcde8599ea905d088f65d90778f86e259b856a49fa5c4ea256ec` |

`java` and `javac` come from `PATH` and must be JDK 17 or newer. The test is compiled with `--release 17`. Each check runs in its own JVM with 10 minutes for `javac` and the JVM together.

A rule that fails prints one line per ArchUnit violation: `<slug>: checks[N] (mode): <rule-name> <message>`. A tree with no class file under the imported packages, a rule whose `that()` selection matches no class, or a layers rule in which every layer matches no class, is an error and is never waived. A layers rule in which only some layers are empty reports one violation for each empty layer. `--report` writes counts and hashes only, never rule names or class names.
