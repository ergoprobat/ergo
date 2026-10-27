package semgrep

import (
	"embed"
	"io/fs"
	"sort"
	"strings"

	"github.com/ipsofacto-io/ipso/internal/compiler"
)

//go:embed templates/*/*.yaml
var templateFiles embed.FS

const templateRoot = "templates"

func lookup(id string) ([]byte, bool) {
	if !compiler.ValidTemplateID(id) {
		return nil, false
	}
	data, err := templateFiles.ReadFile(templateRoot + "/" + id + ".yaml")
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
		if entry.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return nil
		}
		rel := strings.TrimPrefix(path, templateRoot+"/")
		rel = strings.TrimSuffix(rel, ".yaml")
		ids = append(ids, rel)
		return nil
	})
	if err != nil {
		panic("embedded templates: " + err.Error())
	}
	sort.Strings(ids)
	return ids
}
