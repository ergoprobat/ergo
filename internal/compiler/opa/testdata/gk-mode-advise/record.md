---
edr: 1
id: 0MTFDWZZT0JE85JX7V83PFB3VC
slug: gk-mode-advise
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
  - engine: opa
    target: gatekeeper
    ref: gatekeeper.yaml
    mode: advise
waivers: []
sources:
  - type: pr
    url: https://github.com/acme/clusters/pull/12
tags: [fixture]
---

# Advise mode

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
