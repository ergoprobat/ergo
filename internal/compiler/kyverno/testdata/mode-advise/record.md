---
edr: 1
id: 0AC0VXMTE2D0PMYV44ESK2MW1T
slug: mode-advise
title: Advise mode
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
  - engine: kyverno
    ref: policies/run-as-non-root.yaml
    mode: advise
waivers: []
sources:
  - type: pr
    url: https://github.com/acme/clusters/pull/12
tags: [fixture]
---

# Advise mode

## Context

The fixture exercises the kyverno compiler.

## Decision

Compile the admission policy.

## Alternatives considered

- Skip the check.

## Consequences

The golden manifest or the expect file records the result.

## Verification

compilertest compares the compiler output with this directory.
