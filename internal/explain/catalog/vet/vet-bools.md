---
id: vet-bools
source: vet
analyzer: bools
match: ^(?P<kind>redundant|suspect) (?P<op>and|or): (?P<expr>.+)$
title: The condition {{expr}} cannot mean what it says
links:
- The Go spec: Logical operators | https://go.dev/ref/spec#Logical_operators
- The bools check of go vet | https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/bools
---
## What it means

`go vet` calls the condition `{{expr}}` {{kind}}. A suspect condition always has the same result: `x != a || x != b` is always true, because `x` cannot be equal to both. `x == a && x == b` is always false. A redundant condition repeats the same comparison, as in `x == 1 || x == 1`, which often hides a typo in the second part.

## Why Go does this

The compiler evaluates any boolean expression. It does not check whether the result can ever change. The trap comes from negation: "not a and not b" is `x != a && x != b`, and an `||` there turns the condition into "always". `go vet` looks for comparisons of the same variable that make a condition constant or repeated. `go test` runs this check before the tests.

## How to find it

Go to {{file}}:{{line}} and read the condition aloud with real values. Try a value that should pass and one that should fail. Then fix the operator or the operand:

- A suspect `||` with `!=`: you meant "neither a nor b", which is `&&`.
- A suspect `&&` with `==`: you meant "either a or b", which is `||`.
- A redundant part: the repeated comparison was meant to test something else.

## Example

```go
if code != 200 || code != 204 { // always true
	return false
}
```

Use && for "neither 200 nor 204":

```go
if code != 200 && code != 204 {
	return false
}
```

## Learn more
