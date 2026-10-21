---
id: compile-mismatched-untyped
source: compile
match: ^invalid operation: (?P<expr>.+) \(mismatched types (?P<left>.+?) and (?P<right>untyped .+)\)$
match: ^invalid operation: (?P<expr>.+) \(mismatched types (?P<left>untyped [^)]+?) and (?P<right>(?:[^u]|u[^n]|un[^t]).*)\)$
title: In {{expr}}, a constant does not fit the type of the other side
course: 26-logfmt
links:
- The Go spec: Constants | https://go.dev/ref/spec#Constants
- The Go blog: Constants | https://go.dev/blog/constants
- The Go blog: Strings, bytes, runes and characters in Go | https://go.dev/blog/strings
---
## What it means

The expression `{{expr}}` at {{file}}:{{line}} combines `{{left}}` with `{{right}}`. An untyped value such as `"="`, `'a'`, `1.5` or `nil` takes the type of the other side, but only when it can be a value of that type. Here it cannot.

## Why Go does this

Untyped constants make code such as `x * 2` work for any numeric `x`. The constant is converted to the other operand's type, and the conversion must be valid. A string literal is never a byte, a rune literal is never a string, and `nil` is never a struct.

## How to find it

Look at both types in the message:

- `byte` and `untyped string`: indexing a string, `s[i]`, gives a `byte`. Compare it with a rune literal in single quotes: `s[i] == '='`.
- `string` and `untyped rune`: compare a string with a string literal in double quotes, `"="`, or compare `s[i]` with `'='`.
- A number and `untyped string`, as in `n == "1"`: text is not a number. Parse the text with `strconv.Atoi`, or format the number.
- A struct and `untyped nil`: a struct value is never `nil`. Compare with its zero value, `T{}`, or use a pointer.
- `untyped string` and a number in a `+`: build the text with `strconv.Itoa` or `fmt.Sprintf`; `+` does not convert numbers to text.

## Example

```go
for i := 0; i < len(line); i++ {
	if line[i] == "=" { // invalid operation: line[i] == "=" (mismatched types byte and untyped string)
		return line[:i]
	}
}
```

Compare the byte with a rune literal:

```go
	if line[i] == '=' {
```

## Learn more

Exercise 01 practises strings as bytes in a hand-written scanner.
