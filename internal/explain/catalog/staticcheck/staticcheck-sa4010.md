---
id: staticcheck-sa4010
source: staticcheck
code: SA4010
title: The result of append is stored, but nothing reads it
links:
- staticcheck: SA4010 | https://staticcheck.dev/docs/checks/#SA4010
- The Go spec: Appending to and copying slices | https://go.dev/ref/spec#Appending_and_copying_slices
- The Go blog: Arrays, slices (and strings): The mechanics of append | https://go.dev/blog/slices
---
## What it means

You append to a slice variable, but after that no code reads the variable, except other appends to it. The elements that you added are lost. A typical case: a function builds a result slice and then returns its input slice. Another: a function appends to a slice parameter and does not return it.

## Why Go does this

`append` returns a new slice header with the new length, and sometimes a new backing array. Only the variable that you assign it to sees the new elements. A caller's slice keeps its own length, even when it shares the array. The compiler only checks that you assign the result. staticcheck follows the variable and reports that the appends are never read.

## How to find it

Go to {{file}}:{{line}} and find the variable that you append to. Ask where its elements should end up:

- In the result: return that variable, not another one.
- In the caller's slice: return the new slice and let the caller assign it, as with `append` itself.
- In a struct: append to the field, as in `s.items = append(s.items, x)`.

## Example

```go
var out []string
for _, l := range lines {
	if l != "" {
		out = append(out, l)
	}
}
return lines // out is never used
```

Return the slice that you built:

```go
return out
```

## Learn more
