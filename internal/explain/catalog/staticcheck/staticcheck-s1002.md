---
id: staticcheck-s1002
source: staticcheck
code: S1002
match: ^should omit comparison to bool constant, can be simplified to (?P<expr>.+)$
title: The comparison with a bool constant is not needed: write {{expr}}
links:
- staticcheck: S1002 | https://staticcheck.dev/docs/checks/#S1002
- The Go spec: If statements | https://go.dev/ref/spec#If_statements
---
## What it means

The condition compares a boolean with `true` or `false`. A boolean is a condition already: `ok == true` means `ok`, and `ok == false` means `!ok`. The code works, but the extra comparison makes the reader stop and check what it adds. Here, `{{expr}}` says the same.

## Why Go does this

An `if` statement takes any expression of boolean type, and comparing 2 booleans gives a boolean too, so the compiler accepts both forms. staticcheck suggests the short form, because it is what Go code usually looks like and it reads as plain language.

## How to find it

Go to {{file}}:{{line}} and replace the comparison with `{{expr}}`. The short form reads best when the variable has a name that sounds like a condition, such as `healthy`, `done` or `found`. If the name does not read well in an `if`, rename the variable instead of keeping the comparison.

## Example

```go
if healthy == true {
	return "up"
}
```

Use the boolean as the condition:

```go
if healthy {
	return "up"
}
```

## Learn more
