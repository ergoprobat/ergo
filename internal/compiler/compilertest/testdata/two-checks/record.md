---
edr: 1
id: 01M5C5QEHHNXJ3D7X2ZCMGXQT7
slug: two-checks
title: two checks
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
    ref: policies/one.txt
    mode: block
  - engine: opa
    ref: policies/two.txt
    mode: warn
waivers: []
sources:
  - type: pr
    url: https://github.com/acme/platform/pull/10
tags: [fixture]
---

# two checks

## Context

The fixture gives the reference compiler one record to compile.

## Decision

Package the file named by ref.

## Alternatives considered

- Skip the file: the compiler would have nothing to emit.

## Consequences

The artifact bytes match the referenced file.

## Verification

The golden manifest lists the compiled checks, or expect names the reason code.
