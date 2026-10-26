---
edr: 1
id: 01M5WVYED3WMYCRYT7ZA8SZC4G
slug: mode-warn
title: Warn mode does not change the test
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
  - engine: archunit
    ref: archunit.json
    target: jvm
    mode: warn
waivers: []
sources:
  - type: pr
    url: https://github.com/acme/app/pull/51
tags: [fixture]
---

# Warn mode does not change the test

## Context

The fixture exercises the archunit compiler.

## Decision

Compile the architecture rule file.

## Alternatives considered

- Skip the check.

## Consequences

The golden manifest or the expect file records the result.

## Verification

compilertest compares the compiler output with this directory.
