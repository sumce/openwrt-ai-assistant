// Package web embeds the built frontend assets (web/dist).
//
// On a fresh clone, dist/ contains only the placeholder index.html so that
// `go build` works out of the box; run `make frontend` to generate the real
// UI, then rebuild the binary.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var embedded embed.FS

// Dist returns the embedded frontend filesystem rooted at dist/.
func Dist() fs.FS {
	sub, err := fs.Sub(embedded, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
