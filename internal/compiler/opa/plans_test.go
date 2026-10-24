package opa

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ergoprobat/ergo/internal/compiler"
)

// capturedPlans are terraform show -json and tofu show -json output for testdata/plans/main.tf.
// Both create an unencrypted EBS volume and a public MySQL instance.
var capturedPlans = []string{"show-terraform.json", "show-tofu.json"}

func TestCapturedPlans(t *testing.T) {
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	lib := libSource()
	wants := map[string]string{
		"aws/ebs-encrypted":         "aws_ebs_volume.data",
		"aws/rds-not-public":        "aws_db_instance.app",
		"aws/rds-storage-encrypted": "aws_db_instance.app",
		"terraform/postgres-only":   "aws_db_instance.app",
	}
	for _, name := range capturedPlans {
		body, err := os.ReadFile(filepath.Join("testdata", "plans", name))
		if err != nil {
			t.Fatal(err)
		}
		plan, err := ReadPlan(bytes.NewReader(body))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for id, address := range wants {
			t.Run(name+"/"+id, func(t *testing.T) {
				src, ok := lookup(id)
				if !ok {
					t.Fatalf("template %s", id)
				}
				outcome := EvalCheck(compiler.Entry{}, map[string][]byte{
					"policy.rego":    src,
					"terraform.rego": lib,
				}, plan, now)
				if outcome.Err != nil {
					t.Fatal(outcome.Err)
				}
				found := false
				for _, violation := range outcome.Violations {
					if violation.Address == address {
						found = true
					}
				}
				if !found {
					t.Fatalf("no violation for %s in %+v", address, outcome.Violations)
				}
			})
		}
	}
}
