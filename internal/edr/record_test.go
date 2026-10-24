package edr

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestInForce(t *testing.T) {
	from := date(t, "2026-01-15")
	end := date(t, "2026-06-01")
	tests := []struct {
		name   string
		status string
		from   time.Time
		to     *time.Time
		now    time.Time
		want   bool
	}{
		{name: "accepted on the start date", status: "accepted", from: from, now: date(t, "2026-01-15"), want: true},
		{name: "day before the start", status: "accepted", from: from, now: date(t, "2026-01-14"), want: false},
		{name: "day before the end", status: "accepted", from: from, to: &end, now: date(t, "2026-05-31"), want: true},
		{name: "end date is exclusive", status: "accepted", from: from, to: &end, now: date(t, "2026-06-01"), want: false},
		{name: "verified inside the window", status: "verified", from: from, now: date(t, "2026-02-01"), want: true},
		{name: "needs-review inside the window", status: "needs-review", from: from, now: date(t, "2026-02-01"), want: true},
		{name: "proposed", status: "proposed", from: from, now: date(t, "2026-02-01"), want: false},
		{name: "rejected", status: "rejected", from: from, now: date(t, "2026-02-01"), want: false},
		{name: "superseded", status: "superseded", from: from, now: date(t, "2026-02-01"), want: false},
		{name: "deprecated", status: "deprecated", from: from, now: date(t, "2026-02-01"), want: false},
		{
			name:   "local date after UTC has rolled back",
			status: "accepted",
			from:   from,
			now:    time.Date(2026, 1, 15, 0, 30, 0, 0, time.FixedZone("east", 2*60*60)),
			want:   true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rec := Record{Status: test.status, ValidFrom: test.from, ValidTo: test.to}
			if got := rec.InForce(test.now); got != test.want {
				t.Fatalf("InForce() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestWaiverInForce(t *testing.T) {
	from := date(t, "2026-03-01")
	to := date(t, "2026-04-01")
	waiver := Waiver{ValidFrom: from, ValidTo: to}
	tests := []struct {
		name string
		now  time.Time
		want bool
	}{
		{name: "start date included", now: date(t, "2026-03-01"), want: true},
		{name: "day before the start", now: date(t, "2026-02-28"), want: false},
		{name: "day before the end", now: date(t, "2026-03-31"), want: true},
		{name: "end date excluded", now: date(t, "2026-04-01"), want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := waiver.InForce(test.now); got != test.want {
				t.Fatalf("InForce() = %v, want %v", got, test.want)
			}
		})
	}
	inverted := Waiver{ValidFrom: to, ValidTo: from}
	if inverted.InForce(date(t, "2026-03-15")) {
		t.Fatal("a waiver whose start is after its end should never bind")
	}
}

func TestEffectiveMode(t *testing.T) {
	tests := []struct {
		name        string
		mode        string
		criticality string
		want        string
		wantErr     bool
	}{
		{name: "explicit mode wins", mode: "advise", criticality: "blocking", want: "advise"},
		{name: "blocking", criticality: "blocking", want: "block"},
		{name: "warning", criticality: "warning", want: "warn"},
		{name: "advisory", criticality: "advisory", want: "advise"},
		{name: "unknown criticality", criticality: "other", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := (Check{Mode: test.mode}).EffectiveMode(test.criticality)
			if test.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("mode %q, want %q", got, test.want)
			}
		})
	}
}

func TestRecordFromFixture(t *testing.T) {
	raw, err := os.ReadFile("testdata/valid.md")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := Parse(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	rec, err := RecordFrom(doc)
	if err != nil {
		t.Fatal(err)
	}
	if rec.ID != "01M5C5QEHHNXJ3D7X2ZCMGXQT7" || rec.Slug != "tls-on-public-ingress" {
		t.Fatalf("record %+v", rec)
	}
	if rec.ValidTo != nil {
		t.Fatal("valid_to should be open")
	}
	if len(rec.Checks) != 0 {
		t.Fatalf("checks %+v", rec.Checks)
	}
	if !rec.InForce(time.Date(2026, 10, 24, 16, 0, 0, 0, time.UTC)) {
		t.Fatal("fixture should be in force on 2026-10-24")
	}
}

func TestRecordFromChecks(t *testing.T) {
	text := strings.Replace(recordShell(), "checks: []", `checks:
  - engine: opa
    target: terraform-plan
    ref: policies/rule.rego
    mode: warn
  - engine: ci-binding
    job: checkov / CKV_AWS_145`, 1)
	text = strings.Replace(text, "valid_to: null", "valid_to: 2026-06-01", 1)
	doc, err := Parse(text)
	if err != nil {
		t.Fatal(err)
	}
	if problems := Validate(doc); len(problems) != 0 {
		t.Fatal(problems)
	}
	rec, err := RecordFrom(doc)
	if err != nil {
		t.Fatal(err)
	}
	if rec.ValidTo == nil || rec.ValidTo.Format("2006-01-02") != "2026-06-01" {
		t.Fatalf("valid_to %+v", rec.ValidTo)
	}
	if len(rec.Checks) != 2 {
		t.Fatalf("checks %+v", rec.Checks)
	}
	first := rec.Checks[0]
	if first.Engine != "opa" || first.Target != "terraform-plan" || first.Ref != "policies/rule.rego" || first.Mode != "warn" {
		t.Fatalf("first %+v", first)
	}
	second := rec.Checks[1]
	if second.Engine != "ci-binding" || second.Job != "checkov / CKV_AWS_145" || second.Ref != "" {
		t.Fatalf("second %+v", second)
	}
	if len(rec.Scope) != 1 || rec.Scope[0].Env != nil {
		t.Fatalf("scope %+v", rec.Scope)
	}
	if len(rec.Waivers) != 0 {
		t.Fatalf("waivers %+v", rec.Waivers)
	}
}

func TestRecordFromScopeAndWaivers(t *testing.T) {
	text := strings.Replace(recordShell(), "scope:\n  - org: acme", `scope:
  - org: acme
    env: [prod, stage]
  - repo: acme/payments`, 1)
	text = strings.Replace(text, "waivers: []", `waivers:
  - reason: migration window
    approver: team:platform
    valid_from: 2026-03-01
    valid_to: 2026-04-01`, 1)
	doc, err := Parse(text)
	if err != nil {
		t.Fatal(err)
	}
	if problems := Validate(doc); len(problems) != 0 {
		t.Fatal(problems)
	}
	rec, err := RecordFrom(doc)
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.Scope) != 2 || len(rec.Scope[0].Env) != 2 || rec.Scope[1].Env != nil {
		t.Fatalf("scope %+v", rec.Scope)
	}
	if rec.Scope[0].Env[0] != "prod" || rec.Scope[0].Env[1] != "stage" {
		t.Fatalf("env %+v", rec.Scope[0].Env)
	}
	if len(rec.Waivers) != 1 {
		t.Fatalf("waivers %+v", rec.Waivers)
	}
	waiver := rec.Waivers[0]
	if waiver.Approver != "team:platform" || waiver.ValidFrom.Format("2006-01-02") != "2026-03-01" || waiver.ValidTo.Format("2006-01-02") != "2026-04-01" {
		t.Fatalf("waiver %+v", waiver)
	}
}

func date(t *testing.T, text string) time.Time {
	t.Helper()
	parsed, err := time.Parse("2006-01-02", text)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func recordShell() string {
	return `---
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

Every public ingress terminates TLS.

## Alternatives considered

- Keep HTTP for public health checks.

## Consequences

Clients trust the platform certificate.

## Verification

A listener on port 80 that does not redirect fails review.
`
}
