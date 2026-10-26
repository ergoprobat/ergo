---
edr: 1
id: 01M5C5QEHYCBCDSMGYVGQGCRV7
slug: archunit-layers
title: Java services keep the domain free of framework imports
status: accepted
owner: team:platform
scope:
  - org: acme
valid_from: 2026-07-15
valid_to: null
review_by: 2027-01-15
supersedes: []
criticality: blocking
checks:
  - engine: archunit
    ref: archunit.json
    mode: block
waivers: []
sources:
  - type: pr
    url: https://github.com/acme/billing/pull/9
tags: [java]
---

# Java services keep the domain free of framework imports

## Context

A domain type that imports a web framework cannot be tested without starting that framework.

## Decision

The domain package does not import the web or persistence framework. Adapters sit outside the domain.

## Alternatives considered

- A single module with package-by-layer comments: comments do not stop an import.

## Consequences

New framework imports in the domain fail the build.

## Verification

The JSON rule file `archunit.json` compiles to a JUnit Jupiter ArchUnit test. The check is in block mode.
