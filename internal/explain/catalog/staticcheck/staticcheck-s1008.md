---
id: staticcheck-s1008
source: staticcheck
code: S1008
match: ^should use '(?P<want>.+?)' instead of '(?P<have>.+)'$
title: Return the condition itself: {{want}}
links:
- staticcheck: S1008 | https://staticcheck.dev/docs/checks/#S1008
- The Go spec: Comparison operators | https://go.dev/ref/spec#Comparison_operators
---
## What it means

An `if` that returns `true` in 1 branch and `false` after it returns the value of its own condition. The condition is already a boolean, so 1 line says the same: `{{want}}`. The code works, but 4 lines hide what the function computes.

## Why Go does this

A comparison such as `code >= 500` is an expression of type `bool`, like a variable. You can return it, assign it or pass it on, without an `if`. staticcheck suggests the short form, because it states the rule of the function directly.

## How to find it

Go to {{file}}:{{line}} and replace the `if` and the 2 returns with `{{want}}`. When the `if` returns `false` first and `true` after, negate the condition. When the condition gets long, give its parts names with local variables, so the return still reads like the rule it implements.

## Example

```go
if code >= 500 {
	return true
}
return false
```

Return the comparison:

```go
return code >= 500
```

## Learn more
