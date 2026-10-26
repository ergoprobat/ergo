---
edr: 1
id: 086DFHF33T8G75P2QHCQBZB0X4
slug: gk-no-violation
title: no violation
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

# no violation

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
