---
id: vet-nilfunc
source: vet
analyzer: nilfunc
match: ^comparison of function (?P<name>\S+) (?P<op>==|!=) nil is always (?P<result>true|false)$
title: The function {{name}} is never nil, so this comparison is always {{result}}
links:
- The Go spec: Comparison operators | https://go.dev/ref/spec#Comparison_operators
- The Go spec: Function types | https://go.dev/ref/spec#Function_types
- The nilfunc check of go vet | https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/nilfunc
---
## What it means

`{{name}}` is a function that you declared with `func`. A declared function always exists, so comparing it with nil always gives {{result}}. The branch that depends on it either always runs or never runs. Most likely you meant something else.

## Why Go does this

Function values can be compared only with nil. A variable or a struct field of function type is nil until you assign a function to it, so for those the check makes sense. A declared function is never nil. The compiler accepts the comparison anyway, and `go vet` reports it. `go test` runs this check before the tests.

## How to find it

Go to {{file}}:{{line}} and ask what you wanted to test:

- The result of the function: you forgot the parentheses, as in `{{name}}() != nil`.
- An optional hook that a caller may set: then it must be a variable or a field of function type, such as `onEvict func(key string)`, and that is what you compare with nil.

## Example

```go
func defaultLog(msg string) { log.Print(msg) }

if defaultLog != nil { // always true
	defaultLog(msg)
}
```

Make the hook a variable, so that it can be nil:

```go
var logf func(msg string) // nil until someone sets it

if logf != nil {
	logf(msg)
}
```

## Learn more
