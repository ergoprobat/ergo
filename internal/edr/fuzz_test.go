package edr

import "testing"

func FuzzParse(f *testing.F) {
	f.Add("")
	f.Add("---\nslug: x\n---\n")
	f.Fuzz(func(t *testing.T, text string) {
		_, _ = Parse(text)
	})
}
