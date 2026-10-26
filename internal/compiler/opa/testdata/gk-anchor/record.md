---
edr: 1
id: 0H5P7C5MYJ5KTW14ZXSBHC97Z0
slug: gk-anchor
title: anchor
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
  - engine: opa
    target: gatekeeper
    ref: gatekeeper.yaml
    mode: block
waivers: []
sources:
  - type: pr
    url: https://github.com/acme/clusters/pull/12
tags: [fixture]
---

# anchor

## Context

The fixture exercises the gatekeeper compiler.

## Decision

Compile the admission template and constraint.

## Alternatives considered

- Skip the check.

## Consequences

The golden manifest or the expect file records the result.

## Verification

compilertest compares the compiler output with this directory.
