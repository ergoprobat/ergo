// Package gatekeeper evaluates a compiled Gatekeeper check by running the pinned gator CLI on the
// canonical template and constraint that compile wrote, and on a private copy of the repository manifests.
package gatekeeper

const (
	// CodeMissing means gator is not on PATH, or its version command failed.
	CodeMissing = "gatekeeper-missing"
	// CodeVersion means the binary is not the pinned release, or the version output is not the shape ipso reads.
	CodeVersion = "gatekeeper-version"
	// CodePath means a named path is missing, is not a manifest file or a directory, or leaves the
	// repository, or the tree holds a symlink, a file that is not regular, or more manifest bytes than the cap.
	CodePath = "gatekeeper-path"
	// CodeEmpty means the named paths hold no .yaml or .yml file, or no Kubernetes object in them.
	CodeEmpty = "gatekeeper-empty"
	// CodeResource means a document in the named paths is a Gatekeeper object, is a list, or is
	// not something ipso can read the way gator will, so it could change which policy runs or hide an object.
	CodeResource = "gatekeeper-resource"
	// CodeRun means gator did not finish, or exited without a result.
	CodeRun = "gatekeeper-run"
	// CodeOutput means the run produced output ipso cannot trust: something other than the results of
	// this constraint, or results that disagree with the exit status.
	CodeOutput = "gatekeeper-output"
)

const (
	artifactTemplate   = "constraint-template.yaml"
	artifactConstraint = "constraint.yaml"
)
