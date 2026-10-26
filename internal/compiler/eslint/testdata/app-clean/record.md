---
edr: 1
id: 0EH93F4NK2MD0MKT69G065GR9P
slug: app-clean
title: Lint promises and equality
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
    ref: eslint-ts.json
    mode: block
  - engine: eslint
    target: javascript
    ref: eslint-js.json
    mode: block
waivers: []
sources:
  - type: pr
    url: https://github.com/acme/clusters/pull/12
tags: [fixture]
---

# Lint promises and equality

## Context

The fixture is a conformance subject for the eslint compiler.

## Decision

The compiled artifact is evaluated on this repository.

## Alternatives considered

- Skip the check.

## Consequences

A clean subject passes and a violating subject fails.

## Verification

The conformance suite compares the engine verdict with this directory.
