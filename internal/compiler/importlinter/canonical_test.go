package importlinter

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ipsofacto-io/ipso/internal/compiler"
)

func readGolden(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name, "golden", "0", "importlinter.ini"))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func requireNotCanonical(t *testing.T, src string) {
	t.Helper()
	_, err := ReadCanonical([]byte(src))
	var coded *compiler.Error
	if !errors.As(err, &coded) || coded.Code != compiler.CodeBuildManifest {
		t.Fatalf("err %v for\n%s", err, src)
	}
}

func TestReadCanonicalRecoversRootsAndContracts(t *testing.T) {
	got, err := ReadCanonical([]byte(readGolden(t, "shop-violating")))
	if err != nil {
		t.Fatal(err)
	}
	want := Canonical{
		Roots: []string{"shop"},
		Contracts: []Contract{
			{ID: "layers", Name: "Layers", Type: "layers"},
			{ID: "independence", Name: "Independence", Type: "independence"},
			{ID: "forbidden", Name: "Forbidden", Type: "forbidden"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

// Every artifact compile writes for a fixture is its own canonical form.
func TestReadCanonicalAcceptsEveryGolden(t *testing.T) {
	goldens, err := filepath.Glob(filepath.Join("testdata", "*", "golden", "0", "importlinter.ini"))
	if err != nil || len(goldens) == 0 {
		t.Fatalf("goldens %v %v", goldens, err)
	}
	for _, path := range goldens {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ReadCanonical(raw); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
	}
}

func TestReadCanonicalRefusesAnythingElse(t *testing.T) {
	good := readGolden(t, "shop-violating")
	for name, src := range map[string]string{
		"empty":             "",
		"contract class":    strings.Replace(good, "root_packages =\n    shop\n", "contract_types =\n    custom: shop.api.Evil\nroot_packages =\n    shop\n", 1),
		"dotted root":       strings.Replace(good, "    shop\n\n", "    shop.api\n\n", 1),
		"percent":           strings.Replace(good, "name = Layers", "name = 100%", 1),
		"unsorted keys":     strings.Replace(good, "name = Layers\ntype = layers\n", "type = layers\nname = Layers\n", 1),
		"single line list":  strings.Replace(good, "forbidden_modules =\n    shop.infra\n", "forbidden_modules = shop.infra\n", 1),
		"colon delimiter":   strings.Replace(good, "name = Layers", "name: Layers", 1),
		"lowercase boolean": strings.Replace(readGolden(t, "contract-types"), "as_packages = True", "as_packages = true", 1),
		"comment":           "# note\n" + good,
		"crlf":              strings.ReplaceAll(good, "\n", "\r\n"),
		"no final newline":  strings.TrimSuffix(good, "\n"),
		"extra blank line":  good + "\n",
		"unknown key":       strings.Replace(good, "type = layers\n", "type = layers\nunknown = 1\n", 1),
		"no contract":       "[importlinter]\nroot_packages =\n    shop\n",
	} {
		t.Run(name, func(t *testing.T) { requireNotCanonical(t, src) })
	}
}
