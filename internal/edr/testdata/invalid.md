---
edr: 1
id: 01M5C5QEHHNXJ3D7X2ZCMGXQT7
slug: tls-on-public-ingress
title: Public ingress must terminate TLS
status: accepted
owner: team:platform
scope:
  - org: acme
valid_from: 2026-01-15
valid_to: null
review_by: 2026-07-15
supersedes: []
criticality: blocking
checks: []
waivers: []
sources:
  - type: pr
    url: https://github.com/acme/platform/pull/10
tags: [tls]
---

# Public ingress must terminate TLS

## Context

Public entry points that still speak plain HTTP expose tokens and session cookies on the path.

## Decision

Every public ingress terminates TLS. Plain HTTP on a public listener only redirects to HTTPS.

## Alternatives considered

- Keep HTTP for public health checks: those checks do not need a public listener, and a redirect covers browsers.

## Consequences

Clients trust the platform certificate. Private health checks stay off the public port.
