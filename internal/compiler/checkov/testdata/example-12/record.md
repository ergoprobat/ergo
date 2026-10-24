---
edr: 1
id: 01M5C5QEHVBS2Q67NPD1BPZ2EM
slug: admin-port-from-bastion
title: Admin ports are open only from the bastion
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
    ref: CKV_AWS_24
    mode: block
waivers:
  - reason: Vendor appliance still requires a temporary admin listener during cutover.
    approver: Ada Lovelace
    valid_from: 2026-06-01
    valid_to: 2026-06-30
sources:
  - type: pr
    url: https://github.com/acme/platform/pull/70
tags: [network]
---

# Admin ports are open only from the bastion

## Context

Security groups that allow SSH from `0.0.0.0/0` show up in every incident review of this platform.

## Decision

Admin ports accept traffic only from the bastion security group. A time-boxed waiver covers one vendor appliance during cutover.

## Alternatives considered

- A VPN split tunnel instead of a bastion: the split tunnel still lands on laptops the platform does not manage.

## Consequences

The waiver expires on 2026-06-30. After that date the appliance must use the bastion path.

## Verification

Checkov rule `CKV_AWS_24` blocks a security group that opens SSH to the world. The waiver names the approver and the end date.
