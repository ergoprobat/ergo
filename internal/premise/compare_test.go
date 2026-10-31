package premise

import (
	"testing"
)

func TestEqualsComparison(t *testing.T) {
	pass := judge(Spec{Pointer: "/major", Equals: float64(16), HasEquals: true}, []byte(`{"major":16,"secret":"nope"}`))
	if pass.Result != ResultPass || string(pass.Observed) != "16" {
		t.Fatalf("pass = %+v", pass)
	}
	fail := judge(Spec{Pointer: "/major", Equals: float64(16), HasEquals: true}, []byte(`{"major":15}`))
	if fail.Result != ResultFail || string(fail.Observed) != "15" {
		t.Fatalf("fail = %+v", fail)
	}
	wrong := judge(Spec{Pointer: "/major", Equals: float64(16), HasEquals: true}, []byte(`{"major":"16"}`))
	if wrong.Result != ResultError || wrong.Reason != ReasonType {
		t.Fatalf("type = %+v", wrong)
	}
	missing := judge(Spec{Pointer: "/missing", Equals: float64(16), HasEquals: true}, []byte(`{"major":16}`))
	if missing.Result != ResultError || missing.Reason != ReasonPointer || missing.Observed != nil {
		t.Fatalf("missing = %+v", missing)
	}
	notJSON := judge(Spec{Pointer: "/major", Equals: float64(16), HasEquals: true}, []byte(`not-json`))
	if notJSON.Result != ResultError || notJSON.Reason != ReasonNotJSON {
		t.Fatalf("json = %+v", notJSON)
	}
	nullPass := judge(Spec{Pointer: "", Equals: nil, HasEquals: true}, []byte(`null`))
	if nullPass.Result != ResultPass {
		t.Fatalf("null = %+v", nullPass)
	}
	object := judge(Spec{
		Pointer:   "",
		HasEquals: true,
		Equals:    map[string]any{"a": float64(1), "b": float64(2)},
	}, []byte(`{"b":2,"a":1}`))
	if object.Result != ResultPass {
		t.Fatalf("object = %+v", object)
	}
}

func TestPointerEscape(t *testing.T) {
	raw, err := pointerValue([]byte(`{"a/b":1,"arr":[9]}`), "/a~1b")
	if err != nil || string(raw) != "1" {
		t.Fatalf("raw %s err %v", raw, err)
	}
	raw, err = pointerValue([]byte(`{"arr":[9,8]}`), "/arr/0")
	if err != nil || string(raw) != "9" {
		t.Fatalf("index %s err %v", raw, err)
	}
	if _, err := pointerValue([]byte(`{"arr":[9]}`), "/arr/01"); err == nil {
		t.Fatal("leading zero index was accepted")
	}
}

func TestSemverComparison(t *testing.T) {
	cases := []struct {
		constraint string
		body       string
		want       string
		reason     string
	}{
		{">=16.0.0", `{"version":"16.0.0"}`, ResultPass, ""},
		{">=16.0.0", `{"version":"16.1.0"}`, ResultPass, ""},
		{">=16.0.0", `{"version":"15.9.9"}`, ResultFail, ""},
		{">=16.0.0", `{"version":"16.0.0-alpha"}`, ResultFail, ""},
		{">16.0.0", `{"version":"16.0.0"}`, ResultFail, ""},
		{"=16.0.0", `{"version":"16.0.0"}`, ResultPass, ""},
		{"<16.0.0", `{"version":"16.0.0-rc.1"}`, ResultPass, ""},
		{">=16.0.0", `{"version":"16.0.0+build"}`, ResultError, ReasonSemver},
		{">=16.0.0", `{"version":16}`, ResultError, ReasonType},
		{">=16.0.0", `{"version":"01.0.0"}`, ResultError, ReasonSemver},
	}
	for _, test := range cases {
		got := judge(Spec{Pointer: "/version", Semver: test.constraint}, []byte(test.body))
		if got.Result != test.want || got.Reason != test.reason {
			t.Errorf("%s %s = %s %s, want %s %s", test.constraint, test.body, got.Result, got.Reason, test.want, test.reason)
		}
	}
}
