---
id: vet-stdmethods
source: vet
analyzer: stdmethods
match: ^method (?P<have>.+) should have signature (?P<want>.+)$
title: The method {{have}} does not have the signature its name promises
course: 49-diskbuffer
links:
- Package errors | https://pkg.go.dev/errors
- Effective Go: Interface checks | https://go.dev/doc/effective_go#blank_implements
- The stdmethods check of go vet | https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/stdmethods
---
## What it means

The name of this method belongs to a well-known interface of the standard library: `Unwrap`, `Is` and `As` for errors, `MarshalJSON` for encoding/json, `ReadByte` for io, and a few more. The signature must be {{want}}. With a different signature your type does not implement that interface. The code that looks for the method, such as `errors.Is`, `errors.As` or `json.Marshal`, never calls it, and nothing tells you.

## Why Go does this

A type implements an interface when it has every method with exactly the right signature. Nothing is declared. Packages find these methods at run time with a type assertion, so a near miss compiles and is never called. The docs of `errors.Unwrap` say it only calls a method of the form `Unwrap() error`. `go vet` checks the methods whose names match these well-known interfaces.

## How to find it

Go to {{file}}:{{line}}. Compare your signature with {{want}}: the parameter types, the result types and their order. Look the interface up in the docs of its package. To let the compiler check it from now on, add an interface check, as in `var _ json.Marshaler = (*T)(nil)`.

## Example

```go
func (e *ParseError) Unwrap() string { return e.Err.Error() } // not an Unwrap for errors
```

Return the wrapped error itself:

```go
func (e *ParseError) Unwrap() error { return e.Err }
```

## Learn more

The course exercise below practises error types with Is and Unwrap methods.
