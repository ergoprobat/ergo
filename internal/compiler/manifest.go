package compiler

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
)

// Entry is one compiled check in the manifest. The framework fills every field.
type Entry struct {
	DecisionID      string             `json:"decision_id"`
	Slug            string             `json:"slug"`
	CheckIndex      int                `json:"check_index"`
	Engine          string             `json:"engine"`
	Mode            string             `json:"mode"`
	Compiler        string             `json:"compiler"`
	CompilerVersion string             `json:"compiler_version"`
	Artifacts       []ManifestArtifact `json:"artifacts"`
}

// ManifestArtifact is the name and digest of one emitted file.
type ManifestArtifact struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}

type manifestFile struct {
	Entries []Entry `json:"entries"`
}

// Assemble checks artifact names, sorts them, and stamps the manifest entry.
// The returned artifacts are in the same order as the entry.
func Assemble(engine Compiler, in Input, artifacts []Artifact) (Entry, []Artifact, error) {
	mode, err := in.Check.EffectiveMode(in.Record.Criticality)
	if err != nil {
		return Entry{}, nil, err
	}
	ordered := append([]Artifact(nil), artifacts...)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].Name < ordered[j].Name
	})
	seen := make(map[string]struct{}, len(ordered))
	listed := make([]ManifestArtifact, 0, len(ordered))
	for _, artifact := range ordered {
		if !fs.ValidPath(artifact.Name) {
			return Entry{}, nil, &Error{Code: CodeArtifactName, Err: fmt.Errorf("artifact %q", artifact.Name)}
		}
		if _, ok := seen[artifact.Name]; ok {
			return Entry{}, nil, &Error{Code: CodeArtifactName, Err: fmt.Errorf("duplicate artifact %q", artifact.Name)}
		}
		seen[artifact.Name] = struct{}{}
		sum := sha256.Sum256(artifact.Data)
		listed = append(listed, ManifestArtifact{Name: artifact.Name, SHA256: hex.EncodeToString(sum[:])})
	}
	entry := Entry{
		DecisionID:      in.Record.ID,
		Slug:            in.Record.Slug,
		CheckIndex:      in.Index,
		Engine:          in.Check.Engine,
		Mode:            mode,
		Compiler:        engine.Engine(),
		CompilerVersion: engine.Version(),
		Artifacts:       listed,
	}
	return entry, ordered, nil
}

// MarshalManifest encodes entries as deterministic JSON.
// Entries are ordered by decision id, then check index. HTML is not escaped.
func MarshalManifest(entries []Entry) ([]byte, error) {
	ordered := append([]Entry(nil), entries...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].DecisionID != ordered[j].DecisionID {
			return ordered[i].DecisionID < ordered[j].DecisionID
		}
		return ordered[i].CheckIndex < ordered[j].CheckIndex
	})
	for i := range ordered {
		if ordered[i].Artifacts == nil {
			ordered[i].Artifacts = []ManifestArtifact{}
		}
	}
	if ordered == nil {
		ordered = []Entry{}
	}
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(manifestFile{Entries: ordered}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
