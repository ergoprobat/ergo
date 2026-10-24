---
edr: 1
id: 01M5QNFMJMC84HMCRB5XRP16KR
slug: mode-advise
title: Advise mode rewrites dependency-cruiser severity
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
  - engine: depcruise
    ref: .dependency-cruiser.json
    mode: advise
waivers: []
sources:
  - type: pr
    url: https://github.com/acme/app/pull/51
tags: [fixture]
---

# Advise mode rewrites dependency-cruiser severity

## Context

The fixture exercises the dependency-cruiser compiler.

## Decision

Compile the dependency-cruiser rule file.

## Alternatives considered

- Skip the check.

## Consequences

The golden manifest or the expect file records the result.

## Verification

compilertest compares the compiler output with this directory.
