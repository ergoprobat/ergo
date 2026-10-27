package edr

import _ "embed"

// SchemaURL is the canonical name of the embedded draft. The file in this
// package is the artifact to validate against. The URL is not fetched.
const SchemaURL = "https://ipsoplatform.io/schema/edr/v0.1.json"

// SchemaJSON is a copy of ipsofacto-io/edr-spec schema/edr-v0.1.schema.json.
//
//go:embed schema/edr-v0.1.schema.json
var SchemaJSON []byte
