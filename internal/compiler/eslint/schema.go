package eslint

import _ "embed"

// rulesSchemaJSON is the rule file schema.
// The URL inside the file is its id. Compile loads this copy and does not fetch it.
//
//go:embed schema/rules.schema.json
var rulesSchemaJSON []byte
