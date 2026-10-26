---
edr: 1
id: 0JY7S1VR1RSBPJP0GAH16R49GF
slug: javascript
title: JavaScript eqeqeq
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
    target: javascript
    ref: eslint.json
    mode: block
waivers: []
sources:
  - type: pr
    url: https://github.com/acme/app/pull/18
tags: [fixture]
---

# JavaScript eqeqeq

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
