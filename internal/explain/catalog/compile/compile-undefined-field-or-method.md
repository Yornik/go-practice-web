---
id: compile-undefined-field-or-method
source: compile
match: ^(?P<expr>.+?)\.(?P<name>[^.\s]+) undefined \(type (?P<type>.+?) has no field or method [^\s,]+(?:, but does have (?:unexported )?(?:field|method|object) (?P<have>[^\s)]+))?\)$
title: {{type}} has no field or method {{name}}
links:
- The Go spec: Selectors | https://go.dev/ref/spec#Selectors
- The Go spec: Method sets | https://go.dev/ref/spec#Method_sets
- The Go spec: Exported identifiers | https://go.dev/ref/spec#Exported_identifiers
---
## What it means

`{{expr}}` has the type `{{type}}`, and that type has no field and no method named `{{name}}`. When the message ends with `but does have field X` or `but does have method X`, the type has a name that differs only in upper or lower case. For Go these are 2 different names.

## Why Go does this

Go resolves a selector such as `x.f` when it compiles, from the type of `x`: its fields, the fields of embedded structs and its methods. The case of the first letter is part of the name and decides visibility: only upper-case names can be used from another package.

## How to find it

Go to {{file}}:{{line}} and read the type in the message; it is not always the type you had in mind:

- An interface type, such as `error` or `io.Reader`, has only its methods. To reach the fields of the value inside, use a type assertion or `errors.As`.
- A type parameter such as `T` has only the methods of its constraint. Go does not let you use a field through a type parameter.
- A type such as `[]int`, `map[string]int` or `string` has no methods at all. `append`, `len` and `delete` are built-in functions, and the functions for strings are in the package `strings`.
- For a library type, list its API: `go doc time.Time`.

When the error is in a test file, the test uses the exercise's API. Fix the spelling in your code, not in the test.

## Example

```go
type Hit struct {
	Path  string
	Count int
}

for _, h := range hits {
	n += h.count // h.count undefined (type Hit has no field or method count, but does have field Count)
}
```

Use the name as it is declared:

```go
	n += h.Count
```

## Learn more
