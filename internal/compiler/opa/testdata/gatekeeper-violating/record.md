---
edr: 1
id: 0VGY70NNF404A1Q2306VPKGZ9Z
slug: gatekeeper-violating
title: Required owner label
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

# Required owner label

## Context

The fixture is a conformance subject for the gatekeeper compiler.

## Decision

The compiled artifact is evaluated on this repository.

## Alternatives considered

- Skip the check.

## Consequences

A clean subject passes and a violating subject fails.

## Verification

The conformance suite compares the engine verdict with this directory.
