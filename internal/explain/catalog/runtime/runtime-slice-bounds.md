---
id: runtime-slice-bounds
source: runtime
match: ^panic: runtime error: slice bounds out of range \[(?P<bounds>[^\]]*)\](?: with (?:length|capacity) (?P<size>\d+))?$
title: The slice bounds [{{bounds}}] are out of range in {{func}}
links:
- The Go spec: Slice expressions | https://go.dev/ref/spec#Slice_expressions
- The Go blog: Go Slices: usage and internals | https://go.dev/blog/slices-intro
- Package strings: Cut | https://pkg.go.dev/strings#Cut
---
## What it means

At {{file}}:{{line}}, `{{func}}` wrote a slice expression such as `s[low:high]` that Go cannot build. The brackets in the message show the bounds that failed: `[{{bounds}}]`.

## Why Go does this

The spec requires `0 <= low <= high <= len(s)` for strings and arrays. For a slice, the upper limit is the capacity, `cap(s)`, because a slice can grow into the rest of its array. Bounds outside these limits at run time cause a run-time panic.

## How to find it

Read the bounds in the message:

- `[:5] with length 3`: the end is past the end of a string or array. For a slice the message says `capacity`.
- `[4:1]`: the start is after the end. For `s[4:]` the end is the length, so `[4:3]` means the start is past the end.
- `[:-1]` or another negative bound: a search such as `strings.Index` returned -1 for "not found", and the code used it as a position.

Find where each bound comes from on line {{line}}. 2 positions that come from 2 separate searches often do not fit together: the second one can be before the first, or missing. Check every search result for -1 before you use it, and print both bounds and `len(s)` when you are not sure.

## Example

```go
i := strings.IndexByte(s, '(')
j := strings.IndexByte(s, ')')
inner := s[i+1 : j] // j is -1 when there is no ')'
```

Check both results, or let `strings.Cut` do the bookkeeping:

```go
_, rest, ok := strings.Cut(s, "(")
inner, _, ok2 := strings.Cut(rest, ")")
if !ok || !ok2 {
	return "", errors.New("no (...) in input")
}
```

## Learn more
