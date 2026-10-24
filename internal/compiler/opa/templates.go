package opa

import (
	"embed"
	"io/fs"
	"sort"
	"strings"

	"github.com/ergoprobat/ergo/internal/compiler"
)

//go:embed templates
var templateFiles embed.FS

const templateRoot = "templates"

func libSource() []byte {
	data, err := templateFiles.ReadFile(templateRoot + "/lib/terraform.rego")
	if err != nil {
		panic("embedded terraform.rego: " + err.Error())
	}
	return data
}

func lookup(id string) ([]byte, bool) {
	provider, _, _ := strings.Cut(id, "/")
	// lib holds the helper module, which is not a decision template.
	if provider == "lib" || !compiler.ValidTemplateID(id) {
		return nil, false
	}
	data, err := templateFiles.ReadFile(templateRoot + "/" + id + ".rego")
	if err != nil {
		return nil, false
	}
	return data, true
}

func templateIDs() []string {
	var ids []string
	err := fs.WalkDir(templateFiles, templateRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".rego") {
			return nil
		}
		rel := strings.TrimPrefix(path, templateRoot+"/")
		rel = strings.TrimSuffix(rel, ".rego")
		if strings.HasPrefix(rel, "lib/") {
			return nil
		}
		ids = append(ids, rel)
		return nil
	})
	if err != nil {
		panic("embedded templates: " + err.Error())
	}
	sort.Strings(ids)
	return ids
}

func packagePath(id string) string {
	dotted := strings.ReplaceAll(strings.ReplaceAll(id, "-", "_"), "/", ".")
	return "data.ergo.templates." + dotted
}
