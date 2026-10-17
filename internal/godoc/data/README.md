# Standard library index

`stdlib.json.gz` is generated. Do not edit it.

It holds the docs of the Go standard library for the Go release of the runner image, for linux/amd64. Regenerate it with that release:

```bash
GOTOOLCHAIN=go1.27.2 go generate ./internal/godoc
```

When the runner image moves to a new Go release, change `-go` in the `go:generate` line of `godoc.go` in the same change. With another release the generator fails instead of writing docs for the wrong compiler.

The output is deterministic, so it changes in git only when the docs change.
