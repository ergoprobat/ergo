---
edr: 1
id: 01M5QPMHVAP5GADJA353BKZT90
slug: parser
title: parser
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
    mode: block
waivers: []
sources:
  - type: pr
    url: https://github.com/acme/app/pull/51
tags: [fixture]
---

# parser

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
