---
id: staticcheck-sa1019
source: staticcheck
code: SA1019
match: (?s)^(?P<name>\S+) (?:has been|is) deprecated.*$
title: {{name}} is deprecated
links:
- staticcheck: SA1019 | https://staticcheck.dev/docs/checks/#SA1019
- Go Doc Comments: Deprecations | https://go.dev/doc/comment#deprecations
- Go 1 and the Future of Go Programs | https://go.dev/doc/go1compat
---
## What it means

The docs of `{{name}}` mark it as deprecated. It still works, but its authors ask you to stop using it, and the rest of the message quotes their reason and what to use instead. Often the old API has a known flaw. `strings.Title`, for example, does not handle Unicode punctuation correctly.

## Why Go does this

Go keeps old API in the standard library, because the Go 1 compatibility promise says that existing programs keep compiling. Instead of removing a function, its doc comment gets a paragraph that starts with `Deprecated:`. staticcheck reads those paragraphs and reports every use. The documentation in the editor shows the same note.

## How to find it

Read the rest of the message: it names the replacement. Look the replacement up and check 2 things:

- It exists in the Go release of the course module. The course `go.mod` says go 1.24, and `go vet` reports newer functions.
- It is in the standard library. The course allows no other modules, so a replacement from `golang.org/x` is not available. Then write the small part that you need yourself.

## Example

```go
b, err := ioutil.ReadAll(r) // io/ioutil is deprecated
```

Use the function that replaced it:

```go
b, err := io.ReadAll(r)
```

## Learn more
