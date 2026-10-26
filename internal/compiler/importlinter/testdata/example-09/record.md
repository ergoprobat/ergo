---
edr: 1
id: 01M5C5QEHSQ604ZVGMZEQ6RNRC
slug: import-boundaries
title: Services do not import another service's internals
status: accepted
owner: team:platform
scope:
  - org: acme
valid_from: 2026-05-01
valid_to: null
review_by: 2026-11-01
supersedes: []
criticality: advisory
checks:
  - engine: importlinter
    ref: .importlinter
    mode: warn
waivers: []
sources:
  - type: pr
    url: https://github.com/acme/app/pull/30
tags: [architecture]
---

# Services do not import another service's internals

## Context

A cross-service import of an internal module couples deploys that were supposed to move separately.

## Decision

A service imports another service only through its public package. Internal modules stay private.

## Alternatives considered

- A shared kernel package for everything: the kernel becomes a new place for the same coupling.

## Consequences

Import-linter warns while teams finish the split. The mode can move to block after that.

## Verification

The contract is `.importlinter`, evaluated by the importlinter engine in warn mode.
