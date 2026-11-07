// Package frontend embeds the built web app so the Go server can serve it.
package frontend

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var files embed.FS

// Files is the built app, rooted at its folder.
var Files, _ = fs.Sub(files, "dist")
