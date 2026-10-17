---
id: staticcheck-sa4003
source: staticcheck
code: SA4003
match: ^(?:no|every) value of type (?P<type>\S+) is .+$
title: This comparison of a {{type}} always has the same result
course: 24-diskbuffer
links:
- staticcheck: SA4003 | https://staticcheck.dev/docs/checks/#SA4003
- The Go spec: Numeric types | https://go.dev/ref/spec#Numeric_types
- The Go spec: Integer overflow | https://go.dev/ref/spec#Integer_overflow
---
## What it means

The value has type `{{type}}`, and the range of that type decides the comparison before the program runs. An unsigned integer is never less than 0, and a `uint8` is never above 255. So the condition is always true or always false, and the check that you meant to write never happens.

## Why Go does this

Each integer type has a fixed range. Unsigned types have no negative values, and arithmetic on them wraps around: for a `uint32`, `0 - 1` is 4294967295, not -1. So a test such as `if a-b < 0` can never catch a result that went below 0. The compiler accepts the comparison. staticcheck knows the range of each type and reports comparisons that cannot change.

## How to find it

Go to {{file}}:{{line}} and find where the value got its type `{{type}}`:

- A value that you converted from a signed type: check it before the conversion, while it can still be negative.
- A subtraction of unsigned values: compare first, as in `if a < b`, instead of checking the difference.
- A length that came from data, such as a header field: check it against its real limits, such as the size of the input.

## Example

```go
func Valid(size uint32) bool {
	if size < 0 { // a uint32 is never negative
		return false
	}
```

Check the limit that the value can break:

```go
func Valid(size, limit uint32) bool {
	return size <= limit
}
```

## Learn more

The course exercise below practises unsigned lengths in a binary format.
