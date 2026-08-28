package amiflrt

import "embed"

// Files embeds every .go source file in this package so the amifl build
// pipeline can copy amiflrt's real source into a scratch Go module at
// build time (see cmd/amifl's copyAmiflrt) — generated code then imports
// it as an ordinary local package, with no network access or GOPATH
// dependency (doc.go's "独自のGoランタイムを呼ぶ").
//
//go:embed *.go
var Files embed.FS
