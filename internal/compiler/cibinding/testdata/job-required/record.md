---
edr: 1
id: 01JAB3K9ZQ7Y6T0V5M8W2E4R1N
slug: job-required
title: job-required
status: accepted
owner: team:platform
scope:
  - org: acme
valid_from: 2026-11-01
valid_to: null
review_by: 2027-05-01
supersedes: []
criticality: blocking
checks:
  - engine: ci-binding
waivers: []
sources:
  - type: pr
    url: https://github.com/acme/infra/pull/812
tags: [security]
---

# job-required

## Context

The fixture binds a CI job to the decision.

## Decision

The bound job must be green for the same change.

## Alternatives considered

- Skip the binding.

## Consequences

A missing or failed job fails the verdict.

## Verification

The golden manifest records the binding.
