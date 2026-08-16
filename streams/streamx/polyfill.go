package streamx

import _ "embed"

// Source is the bundled streamx-based node classic stream facade.
//
//go:embed bundle.js
var Source string
