---
edr: 1
id: 01M5QV9TJ80AASVVVMSKS4WSV6
slug: template-python-no-shell-true
title: template-python-no-shell-true
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
    ref: ergo:python/no-shell-true
    mode: block
waivers: []
sources:
  - type: pr
    url: https://github.com/acme/app/pull/44
tags: [fixture]
---

# template-python-no-shell-true

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
