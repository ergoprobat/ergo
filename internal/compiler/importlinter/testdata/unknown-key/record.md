---
edr: 1
id: 01M5WVYED08TV32D1DA6J0X3VY
slug: unknown-key
title: unknown-key
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
  - engine: importlinter
    ref: .importlinter
    mode: block
waivers: []
sources:
  - type: pr
    url: https://github.com/acme/app/pull/51
tags: [fixture]
---

# unknown-key

## Context

The fixture exercises the importlinter compiler.

## Decision

Compile the architecture rule file.

## Alternatives considered

- Skip the check.

## Consequences

The golden manifest or the expect file records the result.

## Verification

compilertest compares the compiler output with this directory.
