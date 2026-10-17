---
id: vet-appends
source: vet
analyzer: appends
match: ^append with no values$
title: append gets no values to add
links:
- The Go spec: Appending to and copying slices | https://go.dev/ref/spec#Appending_and_copying_slices
- The Go blog: Arrays, slices (and strings): The mechanics of append | https://go.dev/blog/slices
- The appends check of go vet | https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/appends
---
## What it means

The call at {{file}}:{{line}} passes a slice to `append` and nothing else. `append(s)` returns `s` unchanged, so the line does nothing. Most likely you forgot the value that you meant to add, and the slice stays shorter than you expect.

## Why Go does this

`append` is variadic: after the slice it takes any number of values, and 0 is allowed. So the compiler accepts `append(s)`, and since you use the result, it sees no unused value either. `go vet` reports the call, because adding nothing is almost never what you mean.

## How to find it

Go to {{file}}:{{line}} and ask what should be added:

- 1 or more values: pass them after the slice.
- All elements of another slice: pass that slice with `...`, as in `append(out, more...)`.

Then check the test that failed: a result with too few elements points at the same line.

## Example

```go
if keep(l) {
	out = append(out) // adds nothing
}
```

Pass the value to add:

```go
if keep(l) {
	out = append(out, l)
}
```

## Learn more
