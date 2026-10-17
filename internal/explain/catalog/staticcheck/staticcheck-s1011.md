---
id: staticcheck-s1011
source: staticcheck
code: S1011
match: ^should replace loop with (?P<want>.+)$
title: Append the whole slice at once: {{want}}
links:
- staticcheck: S1011 | https://staticcheck.dev/docs/checks/#S1011
- The Go spec: Passing arguments to ... parameters | https://go.dev/ref/spec#Passing_arguments_to_..._parameters
- The Go spec: Appending to and copying slices | https://go.dev/ref/spec#Appending_and_copying_slices
---
## What it means

The loop appends every element of 1 slice to another, 1 at a time. `append` takes any number of values, and `...` after a slice passes all of its elements at once. So the loop is the same as `{{want}}`. The single call is shorter, and it grows the target slice at most once instead of several times.

## Why Go does this

`append` is variadic. The Go spec says that a slice followed by `...` is passed as the list of values itself, without a copy. The compiler accepts the loop, because each step is valid. staticcheck recognises a loop that only copies elements and suggests the call.

## How to find it

Go to {{file}}:{{line}} and replace the loop with `{{want}}`. Keep the loop when it does more than copy, such as filtering or converting elements. When you combine more than 2 slices, `slices.Concat` builds the result in 1 call.

## Example

```go
for _, s := range b {
	a = append(a, s)
}
```

Append all elements in 1 call:

```go
a = append(a, b...)
```

## Learn more
