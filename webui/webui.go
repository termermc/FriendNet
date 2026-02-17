//go:generate npm ci
//go:generate npm run build

package webui

import "embed"

// Dist contains the embedded web UI files.
//
//go:embed dist/*
var Dist embed.FS
