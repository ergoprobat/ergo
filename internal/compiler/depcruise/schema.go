package depcruise

import _ "embed"

// configurationSchemaJSON is the dependency-cruiser 18.7.0 configuration schema.
// The URL inside the file is its $id. Compile loads this copy and does not fetch it.
//
//go:embed schema/configuration.schema.json
var configurationSchemaJSON []byte
