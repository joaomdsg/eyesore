// Package demoapp is a small, static fixture app used both to try isore out
// manually (cmd/demo) and, later, to drive real e2e tests against a page
// with stable, named elements instead of an inline HTML literal.
package demoapp

import (
	"embed"
	"io/fs"
)

//go:embed all:app
var appFS embed.FS

// FS serves the fixture app's files (index.html and friends) rooted at "/".
func FS() fs.FS {
	sub, err := fs.Sub(appFS, "app")
	if err != nil {
		panic(err) // unreachable: "app" is embedded above
	}
	return sub
}
