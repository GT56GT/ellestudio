//go:build !embedui

package admin

import "io/fs"

// BuiltUI is nil without -tags embedui: plain `go build` and `go test` work
// without Node, and the panel serves the API alone (or ADMIN_UI_DIR).
func BuiltUI() fs.FS { return nil }
