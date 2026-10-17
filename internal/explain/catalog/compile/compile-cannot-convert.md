---
id: compile-cannot-convert
source: compile
match: ^cannot convert (?P<expr>.+?) \((?P<what>.+?)\) to type (?P<type>[^:\n]+)(?:: (?P<reason>.+))?$
title: Go has no conversion from {{expr}} to {{type}}
links:
- The Go spec: Conversions | https://go.dev/ref/spec#Conversions
- Package strconv | https://pkg.go.dev/strconv
- The Go blog: Strings, bytes, runes and characters in Go | https://go.dev/blog/strings
---
## What it means

At {{file}}:{{line}}, Go must convert `{{expr}}` ({{what}}) to `{{type}}`, and no conversion exists between these types. This comes from a conversion such as `int(s)`, or from a `switch` case whose value must have the type of the `switch` expression.

## Why Go does this

A conversion `T(x)` changes the representation between related types: between numeric types, between `string` and `[]byte` or `[]rune`, and between types with the same underlying type. It never parses or formats. Turning `"42"` into `42` can fail, for example for `"4x2"`, so it needs a function that returns an error. A type parameter constrained by `any` can hold any type, so Go allows only the conversions that work for every type in its constraint.

## How to find it

Find what you really want at {{file}}:{{line}}:

- Text to a number: `strconv.Atoi`, `strconv.ParseInt` or `strconv.ParseFloat`, and handle the error.
- A number to text: `strconv.Itoa` or `strconv.FormatFloat`. `string(n)` gives the character with code `n`, not the digits.
- A type parameter `T`: narrow its constraint, for example to `~int | ~float64`. Then `float64(x)` works.
- A `switch` case: the case value must have the type of the `switch` expression. Check that the `{` is on the line of the `switch`.

## Example

```go
func Port(s string) int {
	return int(s) // cannot convert s (variable of type string) to type int
}
```

Parse the text, and pass the error on:

```go
func Port(s string) (int, error) {
	return strconv.Atoi(s)
}
```

## Learn more
