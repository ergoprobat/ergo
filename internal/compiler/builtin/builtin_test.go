package builtin

import "testing"

func TestRegistry(t *testing.T) {
	got, ok := Registry().Lookup("opa")
	if !ok {
		t.Fatal("opa compiler is not registered")
	}
	if got.Engine() != "opa" || got.Version() == "" {
		t.Fatalf("opa compiler %+v", got)
	}
}
