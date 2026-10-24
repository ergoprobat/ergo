package cibinding

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/ergoprobat/ergo/internal/compiler"
)

var (
	shaPattern        = regexp.MustCompile(`^[0-9a-f]{40}$|^[0-9a-f]{64}$`)
	conclusionPattern = regexp.MustCompile(`^[a-z_]{1,32}$`)
)

// ValidCommitSHA reports whether sha is 40 or 64 lowercase hex digits.
func ValidCommitSHA(sha string) bool {
	return shaPattern.MatchString(sha)
}

// Job is one job result in an evidence file.
// Finished is false when conclusion is null.
type Job struct {
	Name       string
	Conclusion string
	Finished   bool
}

// Evidence is one evidence version 1 document.
type Evidence struct {
	SHA  string
	Jobs []Job
	raw  []byte
}

// SHA256 returns the digest of the evidence bytes as they were read.
func (e Evidence) SHA256() string {
	sum := sha256.Sum256(e.raw)
	return hex.EncodeToString(sum[:])
}

// ReadEvidence reads one evidence file.
// wantSHA must equal the sha in the file. A mismatch fails with evidence-sha.
func ReadEvidence(r io.Reader, wantSHA string) (Evidence, error) {
	raw, err := io.ReadAll(io.LimitReader(r, compiler.MaxRefBytes+1))
	if err != nil {
		return Evidence{}, err
	}
	if len(raw) > compiler.MaxRefBytes {
		return Evidence{}, &compiler.Error{Code: CodeEvidenceTooLarge, Err: fmt.Errorf("evidence is larger than %d bytes", compiler.MaxRefBytes)}
	}
	doc, err := decodeEvidence(raw)
	if err != nil {
		return Evidence{}, err
	}
	if doc.SHA != wantSHA {
		return Evidence{}, &compiler.Error{Code: CodeEvidenceSHA, Err: fmt.Errorf("evidence sha %s", doc.SHA)}
	}
	doc.raw = raw
	return doc, nil
}

type evidenceJob struct {
	Name       string          `json:"name"`
	Conclusion json.RawMessage `json:"conclusion"`
}

type evidenceFile struct {
	EvidenceVersion int           `json:"evidence_version"`
	SHA             string        `json:"sha"`
	Jobs            []evidenceJob `json:"jobs"`
}

func decodeEvidence(raw []byte) (Evidence, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var doc evidenceFile
	if err := dec.Decode(&doc); err != nil {
		return Evidence{}, &compiler.Error{Code: CodeEvidenceFormat, Err: fmt.Errorf("expected evidence version 1")}
	}
	if dec.More() {
		return Evidence{}, &compiler.Error{Code: CodeEvidenceFormat, Err: fmt.Errorf("expected one JSON value")}
	}
	if doc.EvidenceVersion != 1 || !shaPattern.MatchString(doc.SHA) || doc.Jobs == nil {
		return Evidence{}, &compiler.Error{Code: CodeEvidenceFormat, Err: fmt.Errorf("expected evidence version 1")}
	}
	jobs := make([]Job, 0, len(doc.Jobs))
	for _, item := range doc.Jobs {
		if !validEvidenceName(item.Name) {
			return Evidence{}, &compiler.Error{Code: CodeEvidenceFormat, Err: fmt.Errorf("job name %q", item.Name)}
		}
		finished, conclusion, err := conclusionOf(item.Conclusion)
		if err != nil {
			return Evidence{}, err
		}
		jobs = append(jobs, Job{Name: item.Name, Conclusion: conclusion, Finished: finished})
	}
	return Evidence{SHA: doc.SHA, Jobs: jobs}, nil
}

func validEvidenceName(name string) bool {
	if name == "" || !utf8.ValidString(name) {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func conclusionOf(raw json.RawMessage) (bool, string, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return false, "", &compiler.Error{Code: CodeEvidenceFormat, Err: fmt.Errorf("job conclusion is missing")}
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return false, "", nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil || !conclusionPattern.MatchString(text) {
		return false, "", &compiler.Error{Code: CodeEvidenceFormat, Err: fmt.Errorf("job conclusion %s", bytes.TrimSpace(raw))}
	}
	return true, text, nil
}

// EvalCheck compares the compiled job name with the evidence.
// A missing job is a violation, and so is each job that did not conclude success.
func EvalCheck(entry compiler.Entry, artifacts map[string][]byte, evidence Evidence, now time.Time) compiler.Outcome {
	job, err := bindingJob(artifacts)
	if err != nil {
		return compiler.Outcome{Err: err}
	}
	var hits []Job
	for _, item := range evidence.Jobs {
		if item.Name == job {
			hits = append(hits, item)
		}
	}
	var violations []compiler.Violation
	if len(hits) == 0 {
		violations = append(violations, compiler.Violation{Message: "job not in evidence: " + job})
	}
	for _, hit := range hits {
		if !hit.Finished {
			violations = append(violations, compiler.Violation{Message: "job has not finished: " + job})
			continue
		}
		if hit.Conclusion != "success" {
			violations = append(violations, compiler.Violation{Message: "job conclusion " + hit.Conclusion + ": " + job})
		}
	}
	return compiler.Cover(entry, compiler.Outcome{Violations: violations}, now)
}

func bindingJob(artifacts map[string][]byte) (string, error) {
	raw, ok := artifacts[artifactName]
	if !ok {
		return "", &compiler.Error{Code: compiler.CodeBuildManifest, Err: fmt.Errorf("missing %s", artifactName)}
	}
	var doc binding
	if err := json.Unmarshal(raw, &doc); err != nil || doc.Job == "" {
		return "", &compiler.Error{Code: compiler.CodeBuildManifest, Err: fmt.Errorf("binding has no job")}
	}
	return doc.Job, nil
}
