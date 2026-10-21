---
id: vet-errorsas
source: vet
analyzer: errorsas
match: ^second argument to errors\.As (?P<problem>must be a non-nil pointer to either a type that implements error, or to any interface type|should not be \*error)$
title: The second argument of errors.As is not a pointer to an error variable
course: 26-logfmt
links:
- Package errors: As | https://pkg.go.dev/errors#As
- The Go blog: Working with Errors in Go 1.13 | https://go.dev/blog/go1.13-errors
- The errorsas check of go vet | https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/errorsas
---
## What it means

`errors.As` searches the chain of an error for an error of a type that you choose, and stores it in a variable of yours. To store it, it needs the address of that variable. The argument here {{problem}}. A value that is not a pointer makes `errors.As` panic when it runs. A pointer to an `error` variable matches every error, so the call always reports true.

## Why Go does this

`errors.As` takes its target as `any`, so the compiler accepts any value. Its docs say that it panics if the target is not a non-nil pointer to a type that implements `error`, or to an interface type. `go vet` checks the type at compile time. `go test` runs this check before the tests.

## How to find it

Go to {{file}}:{{line}} and look at the second argument:

1. Declare a variable of the error type that you look for. For most error types that is a pointer type, such as `*fs.PathError`, because their `Error` method has a pointer receiver.
2. Pass the address of that variable with `&`, even when the variable is a pointer itself.
3. Check the type of the variable: `error` is too wide. Choose the concrete type, or an interface with the method you need.

## Example

```go
var pe *fs.PathError
if errors.As(err, pe) { // pe is nil and not addressed: As panics
```

Pass the address of the variable:

```go
var pe *fs.PathError
if errors.As(err, &pe) {
	return pe.Path
}
```

## Learn more
