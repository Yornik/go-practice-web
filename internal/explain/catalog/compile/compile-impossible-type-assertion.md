---
id: compile-impossible-type-assertion
source: compile
match: ^impossible type (?:assertion|switch case): (?P<expr>[^\n]+)(?:\n\s*(?P<reason>[\s\S]+))?$
title: The type check {{expr}} can never succeed
course: 26-logfmt
links:
- The Go spec: Type assertions | https://go.dev/ref/spec#Type_assertions
- The Go spec: Method sets | https://go.dev/ref/spec#Method_sets
- Package errors: As | https://pkg.go.dev/errors#As
---
## What it means

The type assertion or type switch case `{{expr}}` at {{file}}:{{line}} asks whether an interface holds a value of a type that this interface can never hold. The indented line under the message gives the reason. Most often it ends with `method ... has pointer receiver`: only the pointer type has the method, so the interface can hold a `*T` but never a `T`.

## Why Go does this

For a type assertion `x.(T)` with a concrete type `T`, `T` must implement the interface type of `x`; otherwise `x` could never store a `T`. Go reports such an assertion at compile time, because it would always be false and the code behind it would never run.

## How to find it

Read the reason line, then look at how the methods of the type are declared:

- `has pointer receiver`: test for the pointer type, `x.(*T)` or `case *T:`, because that is what the code that creates the value stores.
- `missing method`: the type does not implement the interface at all. Did you mean another type?

For errors, prefer `errors.As` over a type assertion: it also finds an error that was wrapped with `%w`. Its target is a pointer to a variable of the type you look for.

## Example

```go
func MissingHost(err error) string {
	if nf, ok := err.(NotFoundError); ok { // impossible type assertion: err.(NotFoundError)
		return nf.Host
	}
	return ""
}
```

Look for the pointer type, also in wrapped errors:

```go
	var nf *NotFoundError
	if errors.As(err, &nf) {
		return nf.Host
	}
```

## Learn more

Exercise 01 practises errors.As with a custom error type.
