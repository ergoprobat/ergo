---
edr: 1
id: 01M5C5QEHHNXJ3D7X2ZCMGXQT7
slug: rego-compile
title: rego compile
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
    ref: policies/unsafe.rego
waivers: []
sources:
  - type: pr
    url: https://github.com/acme/platform/pull/10
tags: [fixture]
---

# rego compile

## Context

The fixture exercises the OPA compiler.

## Decision

Compile the Rego check.

## Alternatives considered

- Skip the check.

## Consequences

The golden manifest or the expect file records the result.

## Verification

compilertest compares the compiler output with this directory.
