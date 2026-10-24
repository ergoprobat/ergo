package compiler

import "testing"

func TestValidTemplateID(t *testing.T) {
	ok := []string{"go/http-client-timeout", "lib/terraform", "javascript/no-eval"}
	for _, id := range ok {
		if !ValidTemplateID(id) {
			t.Errorf("%s should be valid", id)
		}
	}
	bad := []string{"", "go", "go/", "/name", "go/name/extra", "Go/name", "go/Name", "go/name.yaml"}
	for _, id := range bad {
		if ValidTemplateID(id) {
			t.Errorf("%s should be invalid", id)
		}
	}
}
