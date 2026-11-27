//go:build embedui

package admin

import (
	"embed"
	"io/fs"
)

// The built UI (admin-ui, `npm run build`) is compiled in with -tags embedui,
// as the Docker image does.
//
//go:embed all:dist
var dist embed.FS

// BuiltUI is the embedded single-page app.
func BuiltUI() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
