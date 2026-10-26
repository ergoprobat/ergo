---
edr: 1
id: 0G0XJK7M3ZSVVGVRK1A6QKJVN1
slug: example-08
title: Example 08
status: accepted
owner: team:platform
scope:
  - org: acme
valid_from: 2026-01-15
valid_to: null
review_by: 2026-07-15
supersedes: []
criticality: warning
checks:
  - engine: eslint
    target: typescript
    ref: eslint.json
    mode: warn
waivers: []
sources:
  - type: pr
    url: https://github.com/acme/app/pull/18
tags: [fixture]
---

# Example 08

## Context

The fixture exercises the eslint compiler.

## Decision

Compile the lint rule file.

## Alternatives considered

- Skip the check.

## Consequences

The golden manifest or the expect file records the result.

## Verification

compilertest compares the compiler output with this directory.
