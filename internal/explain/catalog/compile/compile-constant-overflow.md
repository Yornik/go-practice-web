---
id: compile-constant-overflow
source: compile
match: ^cannot use (?P<value>.+?) \(untyped \w+ constant(?: [^)]+)?\) as (?P<type>\S+) value in .+ \(overflows\)$
match: ^constant (?P<value>\S+) overflows (?P<type>\S+)$
match: ^(?P<value>.+?) \((?:untyped \w+ )?constant(?: [^)]+)?\) overflows (?P<type>\S+)$
title: The constant {{value}} does not fit in the type {{type}}
links:
- The Go spec: Constants | https://go.dev/ref/spec#Constants
- The Go spec: Numeric types | https://go.dev/ref/spec#Numeric_types
- The Go blog: Constants | https://go.dev/blog/constants
---
## What it means

At {{file}}:{{line}}, the constant value `{{value}}` must become a `{{type}}`, and `{{type}}` cannot hold it. For example, a `byte` holds 0 to 255, an `int8` holds -128 to 127, and an unsigned type holds no negative numbers.

## Why Go does this

Go computes constant expressions exactly, with arbitrary precision, while it compiles. Only when a constant gets a concrete type must its value fit that type, and Go checks that at compile time instead of letting the value wrap around. A variable that overflows at run time wraps silently; a constant never does.

## How to find it

Go to {{file}}:{{line}} and find where the constant meets its type: a variable declaration, a conversion such as `byte(x)`, an argument or an arithmetic expression with a typed constant.

- Is the type too small? Choose a type with a larger range, such as `int` or `uint16`.
- Is the value wrong? Count the bits of a shift: `1 << 8` is 256, 1 more than a `byte` holds.
- Did you mean to keep only the low bits, as in a byte encoding? Do it with a variable and a mask, such as `byte(n & 0xff)`. A conversion of a variable wraps without an error.
- With large typed constants, such as `limit * 4`, the overflow happens in the expression itself.

## Example

```go
func Header() []byte {
	var flags byte = 256 // cannot use 256 (untyped int constant) as byte value in variable declaration (overflows)
	return []byte{flags}
}
```

Keep the value inside the range of the type:

```go
	var flags byte = 0x80
```

## Learn more
