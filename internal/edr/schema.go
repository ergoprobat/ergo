package edr

import _ "embed"

// SchemaURL is the canonical name of the embedded draft. The file in this
// package is the artifact to validate against. The URL is not fetched.
const SchemaURL = "https://ipsoplatform.io/schema/edr/v0.2.json"

// SchemaJSON is a copy of ipsofacto-io/edr-spec schema/edr-v0.2.schema.json.
// Draft 0.2 accepts a 0.1 file and adds an optional premises array.
//
//go:embed schema/edr-v0.2.schema.json
var SchemaJSON []byte
