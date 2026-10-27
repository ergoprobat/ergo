---
edr: 1
id: 01M5YAGXHX5P67MEJ8QPEHAE9D
slug: option-interfile
title: option-interfile
status: accepted
owner: team:platform
scope:
  - org: acme
valid_from: 2026-01-15
valid_to: null
review_by: 2026-07-15
supersedes: []
criticality: blocking
checks:
  - engine: semgrep
    ref: rules/sample.yaml
    mode: warn
waivers: []
sources:
  - type: pr
    url: https://github.com/acme/app/pull/44
tags: [fixture]
---

# option-interfile

## Context

The fixture exercises the Semgrep compiler.

## Decision

Compile the Semgrep rule.

## Alternatives considered

- Skip the check.

## Consequences

The golden manifest or the expect file records the result.

## Verification

compilertest compares the compiler output with this directory.
