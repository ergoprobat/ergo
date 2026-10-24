package opa

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/ergoprobat/ergo/internal/compiler"
	"github.com/ergoprobat/ergo/internal/edr"
)

func TestDeniedBuiltins(t *testing.T) {
	cases := map[string]string{
		"http.send":          "resp := http.send({\"method\": \"GET\", \"url\": \"https://example.com\"})\n\tmsg := sprintf(\"%v\", [resp])",
		"net.lookup_ip_addr": "addrs := net.lookup_ip_addr(\"example.com\")\n\tmsg := sprintf(\"%v\", [addrs])",
		"opa.runtime":        "info := opa.runtime()\n\tmsg := sprintf(\"%v\", [info])",
	}
	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			src := "package demo\n\nimport rego.v1\n\ndeny contains msg if {\n\t" + call + "\n}\n"
			err := compileRepoSource(t, src)
			if compiler.CodeOf(err) != CodeRegoCompile {
				t.Fatalf("code %q err %v", compiler.CodeOf(err), err)
			}
		})
	}
}

func TestParseDepth(t *testing.T) {
	var b strings.Builder
	b.WriteString("package demo\n\nimport rego.v1\n\ndeny contains msg if {\n\tmsg := ")
	const depth = maxParseDepth + 8
	for range depth {
		b.WriteByte('[')
	}
	b.WriteString(`"x"`)
	for range depth {
		b.WriteByte(']')
	}
	b.WriteString("\n}\n")
	err := compileRepoSource(t, b.String())
	if compiler.CodeOf(err) != CodeRegoParse {
		t.Fatalf("code %q err %v", compiler.CodeOf(err), err)
	}
}

func TestExprBudget(t *testing.T) {
	var b strings.Builder
	b.WriteString("package demo\n\nimport rego.v1\n\n")
	for range maxExprs + 1 {
		b.WriteString("deny contains msg if {\n\tmsg := \"x\"\n}\n")
	}
	err := compileRepoSource(t, b.String())
	if compiler.CodeOf(err) != CodeRegoCompile || !strings.Contains(err.Error(), "expressions") {
		t.Fatalf("code %q err %v", compiler.CodeOf(err), err)
	}
}

func TestDeepPathAndModule(t *testing.T) {
	policy := `package ergo.templates.test.deep

import data.ergo.lib.terraform as tf

deny contains tf.violation(rc, msg) if {
	some rc in tf.resources("demo_resource")
	msg := tf.require_msg(rc, ["a", "b", "c", "d", "e", "f", "g"], "ok", "g")
}

deny contains tf.violation(bucket, msg) if {
	some bucket in tf.resources("aws_s3_bucket")
	not paired(bucket)
	msg := sprintf("%s unpaired", [bucket.address])
}

paired(bucket) if {
	some enc in tf.resources("aws_s3_bucket_server_side_encryption_configuration")
	tf.same_bucket(enc, bucket)
}
`
	lib := string(libSource())
	known := `{
	  "resource_changes": [{
	    "address": "demo_resource.x",
	    "mode": "managed",
	    "type": "demo_resource",
	    "change": {"actions": ["create"], "after": {"a": {"b": {"c": {"d": {"e": {"f": {"g": "ok"}}}}}}}}
	  }]
	}`
	if got := denials(t, "data.ergo.templates.test.deep", policy, lib, []byte(known)); len(got) != 0 {
		t.Fatalf("deep known value denied %#v", got)
	}
	unknown := `{
	  "resource_changes": [{
	    "address": "demo_resource.x",
	    "mode": "managed",
	    "type": "demo_resource",
	    "change": {
	      "actions": ["create"],
	      "after": {"a": {"b": {"c": {"d": {"e": {"f": {"g": null}}}}}}},
	      "after_unknown": {"a": {"b": {"c": {"d": {"e": {"f": {"g": true}}}}}}}
	    }
	  }]
	}`
	got := denials(t, "data.ergo.templates.test.deep", policy, lib, []byte(unknown))
	msgs := got["demo_resource.x"]
	if len(msgs) == 0 || !strings.Contains(msgs[0], "unknown") {
		t.Fatalf("deep unknown %#v", got)
	}

	const deep = "module.a.module.b.module.c.module.d.module.e"
	const other = "module.a.module.b.module.c.module.d.module.f"
	plan := `{
	  "resource_changes": [
	    {"address": "` + deep + `.aws_s3_bucket.logs", "mode": "managed", "type": "aws_s3_bucket", "change": {"actions": ["create"], "after": {"bucket": "logs", "id": "logs"}}},
	    {"address": "` + deep + `.aws_s3_bucket.data", "mode": "managed", "type": "aws_s3_bucket", "change": {"actions": ["create"], "after": {"bucket": "data", "id": "data"}}},
	    {"address": "` + other + `.aws_s3_bucket_server_side_encryption_configuration.logs", "mode": "managed", "type": "aws_s3_bucket_server_side_encryption_configuration", "change": {"actions": ["create"], "after": {"bucket": "logs", "rule": [{"apply_server_side_encryption_by_default": [{"sse_algorithm": "aws:kms", "kms_master_key_id": "alias/custom"}]}]}}},
	    {"address": "` + deep + `.aws_s3_bucket_server_side_encryption_configuration.data", "mode": "managed", "type": "aws_s3_bucket_server_side_encryption_configuration", "change": {"actions": ["create"], "after": {"bucket": "data", "rule": [{"apply_server_side_encryption_by_default": [{"sse_algorithm": "aws:kms", "kms_master_key_id": "alias/custom"}]}]}}}
	  ]
	}`
	got = denials(t, "data.ergo.templates.test.deep", policy, lib, []byte(plan))
	if _, ok := got[deep+".aws_s3_bucket.data"]; ok {
		t.Fatalf("same-module bucket denied %#v", got)
	}
	if _, ok := got[deep+".aws_s3_bucket.logs"]; !ok {
		t.Fatalf("fifth module segment was collapsed %#v", got)
	}
}

func compileRepoSource(t *testing.T, src string) error {
	t.Helper()
	repo := fstest.MapFS{"policies/rule.rego": &fstest.MapFile{Data: []byte(src)}}
	_, err := New().Compile(compiler.Input{
		Check: edr.Check{Engine: "opa", Ref: "policies/rule.rego"},
		Repo:  repo,
	})
	return err
}
