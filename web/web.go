// Package web embeds the site's templates and static files, so the web
// server image needs nothing beside its binary and the course. In dev mode
// cmd/web reads the same directories from disk instead.
package web

import (
	"embed"
	"io/fs"
)

//go:embed templates
var templates embed.FS

// all: keeps files whose names start with "_" or ".", which Monaco's
// build may contain.
//
//go:embed all:static
var static embed.FS

// Templates returns web/templates.
func Templates() fs.FS { return sub(templates, "templates") }

// Static returns web/static.
func Static() fs.FS { return sub(static, "static") }

func sub(fsys fs.FS, dir string) fs.FS {
	s, err := fs.Sub(fsys, dir)
	if err != nil {
		// The directories are embedded above; fs.Sub fails only on an
		// invalid name.
		panic(err)
	}
	return s
}
