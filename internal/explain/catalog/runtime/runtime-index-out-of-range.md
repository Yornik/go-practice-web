---
id: runtime-index-out-of-range
source: runtime
match: ^panic: runtime error: index out of range \[(?P<index>-?\d+)\](?: with length (?P<length>\d+))?$
title: Index {{index}} is out of range in {{func}}
links:
- The Go spec: Index expressions | https://go.dev/ref/spec#Index_expressions
- The Go blog: Go Slices: usage and internals | https://go.dev/blog/slices-intro
- Effective Go: Slices | https://go.dev/doc/effective_go#slices
---
## What it means

At {{file}}:{{line}}, `{{func}}` used index `{{index}}` on a slice, an array or a string that has no element there. Valid indexes go from 0 to the length minus 1. When the index is not negative, the message also gives the length.

## Why Go does this

Go checks every index at run time. The spec says that an index out of range at run time causes a run-time panic. The check turns a silent read of the wrong memory into a crash at the exact line, so you find the bug where it happens.

## How to find it

Compare the index with the length in the message:

- The index equals the length, such as `[2] with length 2`: an off-by-one. Look for `s[len(s)]`, a loop with `<=`, or a position right after the last element.
- The index is `-1`: `len(s)-1` on an empty slice, or a search such as `strings.Index` that returned -1 for "not found".
- The length is 0: the slice is empty. Was it filled? `make([]T, 0, n)` has length 0, and an `append` to another variable does not grow this one.
- The first frames are in the standard library, such as `encoding/binary`: you passed a slice that is too short. Find the first frame in the exercise package.

Then print `len(s)` and the index right before the line, and work through the empty and the 1-element input by hand.

## Example

```go
func Last(xs []int) int {
	return xs[len(xs)] // index out of range [2] with length 2
}
```

The last element is at `len(xs)-1`, and an empty slice has none:

```go
func Last(xs []int) (int, bool) {
	if len(xs) == 0 {
		return 0, false
	}
	return xs[len(xs)-1], true
}
```

## Learn more
