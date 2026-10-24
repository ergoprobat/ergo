package compiler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ergoprobat/ergo/internal/edr"
)

const (
	// CodeBuildMissing means .ergo/build is absent or has no manifest.
	CodeBuildMissing = "build-missing"
	// CodeBuildDigest means an artifact does not match the digest in the manifest.
	CodeBuildDigest = "build-digest"
	// CodeBuildManifest means the manifest cannot be read or carries a field the evaluator cannot use.
	CodeBuildManifest = "build-manifest"

	planTarget = "terraform-plan"
	opaEngine  = "opa"
)

// Build is one compiled .ergo/build directory.
// Files holds the artifact bytes verified against the manifest, keyed by decision id and check index.
type Build struct {
	Root     string
	Manifest []byte
	Entries  []Entry
	Files    map[string]map[string][]byte
}

// SHA256 returns the digest of the manifest bytes.
func (b Build) SHA256() string {
	sum := sha256.Sum256(b.Manifest)
	return hex.EncodeToString(sum[:])
}

// LoadBuild reads .ergo/build and checks every artifact against its manifest digest.
// A symlink at .ergo/build is refused, and every manifest path must stay inside the build.
func LoadBuild(root string) (Build, error) {
	dir := filepath.Join(root, filepath.FromSlash(buildDir))
	if err := refuseSymlink(dir); err != nil {
		return Build{}, err
	}
	buildRoot, err := os.OpenRoot(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return Build{}, &Error{Code: CodeBuildMissing, Err: fmt.Errorf("run ergo compile first")}
	}
	if err != nil {
		return Build{}, err
	}
	defer func() { _ = buildRoot.Close() }()
	raw, err := fs.ReadFile(buildRoot.FS(), "manifest.json")
	if errors.Is(err, fs.ErrNotExist) {
		return Build{}, &Error{Code: CodeBuildMissing, Err: fmt.Errorf("run ergo compile first")}
	}
	if err != nil {
		return Build{}, err
	}
	var decoded manifestFile
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return Build{}, &Error{Code: CodeBuildManifest, Err: err}
	}
	files := make(map[string]map[string][]byte, len(decoded.Entries))
	for _, entry := range decoded.Entries {
		if _, ok := files[entryKey(entry)]; ok {
			return Build{}, &Error{Code: CodeBuildManifest, Err: fmt.Errorf("decision %s check %d listed twice", entry.DecisionID, entry.CheckIndex)}
		}
		if err := entry.dates(); err != nil {
			return Build{}, &Error{Code: CodeBuildManifest, Err: err}
		}
		got := make(map[string][]byte, len(entry.Artifacts))
		for _, artifact := range entry.Artifacts {
			if _, err := artifactPath(dir, entry.DecisionID, entry.CheckIndex, artifact.Name); err != nil {
				return Build{}, err
			}
			rel := filepath.ToSlash(filepath.Join(entry.DecisionID, fmt.Sprintf("%d", entry.CheckIndex), filepath.FromSlash(artifact.Name)))
			body, err := readArtifact(buildRoot.FS(), rel)
			if err != nil {
				return Build{}, &Error{Code: codeOfRead(err), Err: fmt.Errorf("%s/%d/%s: %w", entry.DecisionID, entry.CheckIndex, artifact.Name, err)}
			}
			sum := sha256.Sum256(body)
			if hex.EncodeToString(sum[:]) != artifact.SHA256 {
				return Build{}, &Error{Code: CodeBuildDigest, Err: fmt.Errorf("%s/%d/%s", entry.DecisionID, entry.CheckIndex, artifact.Name)}
			}
			got[artifact.Name] = body
		}
		files[entryKey(entry)] = got
	}
	return Build{Root: dir, Manifest: raw, Entries: decoded.Entries, Files: files}, nil
}

func readArtifact(build fs.FS, rel string) ([]byte, error) {
	info, err := fs.Stat(build, rel)
	if err != nil {
		return nil, err
	}
	if info.Size() > MaxRefBytes {
		return nil, fmt.Errorf("artifact is %d bytes", info.Size())
	}
	body, err := fs.ReadFile(build, rel)
	if err != nil {
		return nil, err
	}
	if len(body) > MaxRefBytes {
		return nil, fmt.Errorf("artifact is %d bytes", len(body))
	}
	return body, nil
}

func codeOfRead(err error) string {
	if strings.Contains(err.Error(), "escapes from parent") {
		return CodeArtifactName
	}
	if strings.Contains(err.Error(), "artifact is") {
		return CodeRefTooLarge
	}
	return CodeBuildDigest
}

func entryKey(entry Entry) string {
	return fmt.Sprintf("%s/%d", entry.DecisionID, entry.CheckIndex)
}

// dates reports whether the entry carries dates the evaluator can use.
func (e Entry) dates() error {
	if _, err := time.Parse(dateLayout, e.ValidFrom); err != nil {
		return fmt.Errorf("valid_from %q", e.ValidFrom)
	}
	if e.ValidTo != nil {
		if _, err := time.Parse(dateLayout, *e.ValidTo); err != nil {
			return fmt.Errorf("valid_to %q", *e.ValidTo)
		}
	}
	for _, waiver := range e.Waivers {
		if _, err := time.Parse(dateLayout, waiver.ValidFrom); err != nil {
			return fmt.Errorf("waiver valid_from %q", waiver.ValidFrom)
		}
		if _, err := time.Parse(dateLayout, waiver.ValidTo); err != nil {
			return fmt.Errorf("waiver valid_to %q", waiver.ValidTo)
		}
	}
	return nil
}

// Artifacts returns a copy of the bytes LoadBuild verified. It does not read the build again.
func (b Build) Artifacts(entry Entry) (map[string][]byte, error) {
	cached, ok := b.Files[entryKey(entry)]
	if !ok {
		return nil, &Error{Code: CodeBuildDigest, Err: fmt.Errorf("decision %s check %d", entry.DecisionID, entry.CheckIndex)}
	}
	out := make(map[string][]byte, len(cached))
	for name, body := range cached {
		out[name] = append([]byte(nil), body...)
	}
	return out, nil
}

// Selected reports whether the entry is evaluated for env on the calendar date of now.
// An entry applies when it is an opa check for a Terraform plan, its decision is in force,
// and env is empty or the decision names that environment.
func (e Entry) Selected(env string, now time.Time) bool {
	if e.Engine != opaEngine || e.Target != planTarget {
		return false
	}
	from, err := time.Parse(dateLayout, e.ValidFrom)
	if err != nil {
		return false
	}
	var to *time.Time
	if e.ValidTo != nil {
		parsed, err := time.Parse(dateLayout, *e.ValidTo)
		if err != nil {
			return false
		}
		to = &parsed
	}
	// Compile already dropped records that were not in force, so the status here only
	// satisfies the date rule.
	if !(edr.Record{Status: "accepted", ValidFrom: from, ValidTo: to}).InForce(now) {
		return false
	}
	if env == "" || e.Env == nil {
		return true
	}
	return contains(e.Env, env)
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// Verdict is the outcome of one evaluated check.
type Verdict struct {
	DecisionID string `json:"decision_id"`
	Slug       string `json:"slug"`
	CheckIndex int    `json:"check_index"`
	Mode       string `json:"mode"`
	Result     string `json:"result"`
	Violations int    `json:"violations"`
}

// PlanReport is the local verdict report. It carries counts and hashes, never plan contents.
type PlanReport struct {
	ReportVersion int       `json:"report_version"`
	ErgoVersion   string    `json:"ergo_version"`
	EvaluatedAt   time.Time `json:"evaluated_at"`
	Env           string    `json:"env"`
	PlanSHA256    string    `json:"plan_sha256"`
	BuildSHA256   string    `json:"build_sha256"`
	Verdicts      []Verdict `json:"verdicts"`
}

// ReportVersion is the report contract version.
const ReportVersion = 1

// MarshalReport encodes a report as deterministic JSON.
// evaluated_at is UTC with a trailing Z and no fractional seconds.
func MarshalReport(report PlanReport) ([]byte, error) {
	report.EvaluatedAt = report.EvaluatedAt.UTC().Truncate(time.Second)
	if report.Verdicts == nil {
		report.Verdicts = []Verdict{}
	}
	sort.Slice(report.Verdicts, func(i, j int) bool {
		if report.Verdicts[i].DecisionID != report.Verdicts[j].DecisionID {
			return report.Verdicts[i].DecisionID < report.Verdicts[j].DecisionID
		}
		return report.Verdicts[i].CheckIndex < report.Verdicts[j].CheckIndex
	})
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}
