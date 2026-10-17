---
id: staticcheck-sa4023
source: staticcheck
code: SA4023
match: ^this comparison is (?P<result>never|always) true(?:;.*)?$
title: An interface that holds a nil pointer is not nil, so this comparison is {{result}} true
links:
- staticcheck: SA4023 | https://staticcheck.dev/docs/checks/#SA4023
- Go FAQ: Why is my nil error value not equal to nil? | https://go.dev/doc/faq#nil_error
- The Go blog: The Laws of Reflection | https://go.dev/blog/laws-of-reflection
---
## What it means

The left side is an interface value, such as an `error`, that got its value from an expression of a concrete type, such as `*ValidationError`. An interface is nil only when it holds no type and no value. A nil `*ValidationError` stored in an `error` holds a type, so it is not nil. The comparison with nil is {{result}} true, and the caller sees an error even when nothing went wrong.

## Why Go does this

An interface value has 2 parts: a dynamic type and a value of that type. The Go FAQ explains that storing a nil pointer in an interface sets the type part, so the interface is no longer nil. The compiler accepts the comparison, because it is valid for interfaces. staticcheck follows where the value came from and sees that its type is always set.

## How to find it

Go to {{file}}:{{line}} and find where the left side got its value: usually an assignment from a function or a variable with a concrete pointer type. Fix it at that source:

- A function that can fail returns the type `error`, not a concrete error type, and it returns a literal `nil` on success.
- Do not store a typed pointer in an `error` variable before you know that it is an error.

## Example

```go
func validate(name string) *ValidationError { ... } // returns a nil pointer on success

var err error = validate(name)
if err == nil { // never true
```

Return the interface type, and a plain nil on success:

```go
func validate(name string) error {
	if name == "" {
		return &ValidationError{Field: "name"}
	}
	return nil
}
```

## Learn more
