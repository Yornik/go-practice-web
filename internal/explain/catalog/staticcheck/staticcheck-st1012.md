---
id: staticcheck-st1012
source: staticcheck
code: ST1012
match: ^error var (?P<name>\S+) should have name of the form (?P<form>\w+)$
title: The error variable {{name}} should have a name of the form {{form}}
links:
- staticcheck: ST1012 | https://staticcheck.dev/docs/checks/#ST1012
- Package errors | https://pkg.go.dev/errors
- The Go blog: Working with Errors in Go 1.13 | https://go.dev/blog/go1.13-errors
---
## What it means

`{{name}}` is a package-level error variable: a sentinel error that callers compare with `errors.Is`. By convention its name starts with `Err` when it is exported and with `err` when it is not, as in `fs.ErrNotExist`. A name in the form {{form}} tells every reader at once that the value is an error to check for.

## Why Go does this

The standard library names its sentinel errors this way, such as `io.ErrUnexpectedEOF` and `os.ErrDeadlineExceeded`. A few old ones, such as `io.EOF` and `context.Canceled`, are the exceptions. The compiler accepts any name. staticcheck checks the convention for error variables declared at package level.

## How to find it

Go to {{file}}:{{line}} and rename `{{name}}` to the form {{form}}, then update every use. When the spec or a test already names the variable, keep that name: the tests refer to it.

## Example

```go
var NotFound = errors.New("not found")
```

Start the name with Err:

```go
var ErrNotFound = errors.New("not found")
```

## Learn more
