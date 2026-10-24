---
edr: 1
id: 01JAB3K9ZQ7Y6T0V5M8W2E4R1N
slug: s3-kms-cmk-required
title: S3 buckets in production must use KMS customer-managed keys
status: accepted
owner: team:platform
scope:
  - org: acme
    repo: acme/infra-*
    paths:
      - terraform/**
    env:
      - prod
valid_from: 2026-11-01
valid_to: null
review_by: 2027-05-01
supersedes: [s3-sse-s3-allowed]
criticality: blocking
checks:
  - engine: opa
    target: terraform-plan
    ref: policies/s3_kms.rego
  - engine: ci-binding
    job: checkov / CKV_AWS_145
waivers: []
sources:
  - type: pr
    url: https://github.com/acme/infra/pull/812
tags: [security, dora-art9]
---

# S3 buckets in production must use KMS customer-managed keys

## Context

SSE-S3 does not let the platform rotate or disable a key when a bucket holds regulated data. The previous record `s3-sse-s3-allowed` covered the period before customer-managed keys existed.

## Decision

Production S3 buckets use a KMS customer-managed key. This record replaces `s3-sse-s3-allowed`.

## Alternatives considered

- Stay on SSE-S3 and log key use: the platform still cannot disable the key, so a leak keeps reading data.

## Consequences

Plans that create a production bucket without the key fail. Existing buckets are listed and migrated.

## Verification

OPA policy `policies/s3_kms.rego` checks the Terraform plan, and the CI job `checkov / CKV_AWS_145` must be green for the same change.
