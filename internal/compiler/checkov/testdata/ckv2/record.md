---
edr: 1
id: 01M5C5QEHVBS2Q67NPD1BPZ2EM
slug: graph-check
title: graph-check
status: accepted
owner: team:security
scope:
  - org: acme
valid_from: 2026-06-01
valid_to: null
review_by: 2026-12-01
supersedes: []
criticality: blocking
checks:
  - engine: checkov
    ref: CKV2_AWS_6
waivers: []
sources:
  - type: pr
    url: https://github.com/acme/platform/pull/70
tags: [network]
---

# graph-check

## Context

The fixture binds a Checkov check id to the decision.

## Decision

The Checkov check must pass for the same change.

## Alternatives considered

- Skip the binding.

## Consequences

A failed, skipped, or unreported check fails the verdict.

## Verification

The golden manifest records the check id.
