// Package config holds first-party YAML catalogues as bytes.
//
// This package imports stdlib only (draft 32). Parsing and Caps live in
// the owning domain package — org plans in internal/orgplan.
package config

import _ "embed"

// YAML is the hosted organization-plan catalogue. Slugs are map keys.
// There is no label; UI and site copy map the slug (draft 48).
//
//go:embed catalog.yaml
var YAML []byte
