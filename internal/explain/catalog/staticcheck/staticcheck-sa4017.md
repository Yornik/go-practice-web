---
id: staticcheck-sa4017
source: staticcheck
code: SA4017
match: ^(?P<func>\S+) doesn't have side effects and its return value is ignored$
title: The call to {{func}} does nothing, because its result is thrown away
links:
- staticcheck: SA4017 | https://staticcheck.dev/docs/checks/#SA4017
- The Go spec: String types | https://go.dev/ref/spec#String_types
- Package strings | https://pkg.go.dev/strings
---
## What it means

`{{func}}` changes nothing; it only returns a result. You call it as a statement and drop that result, so the line has no effect. For example, `strings.TrimSpace(s)` does not trim `s`: it returns a trimmed copy, and `s` stays as it was.

## Why Go does this

The Go spec says that strings are immutable: once created, a string cannot change. So every function in package strings returns a new string instead of changing its argument. The same holds for many other functions that compute a value. Go allows a call as a statement and drops the result, so the compiler accepts it. staticcheck knows which functions have no side effects.

## How to find it

Go to {{file}}:{{line}} and assign the result, usually to the same variable: `s = {{func}}(s)` for a function of package strings. Then check the lines around it for the same pattern: `strings.ToLower`, `strings.Replace` and `strings.TrimPrefix` also return their result.

## Example

```go
strings.TrimSpace(s) // s keeps its spaces
return s
```

Use the returned string:

```go
s = strings.TrimSpace(s)
return s
```

## Learn more
