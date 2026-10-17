---
id: runtime-makeslice
source: runtime
match: ^panic: runtime error: makeslice: (?P<which>len|cap) out of range$
title: make got an impossible slice {{which}} in {{func}}
links:
- The Go spec: Making slices, maps and channels | https://go.dev/ref/spec#Making_slices_maps_and_channels
- Package builtin: make | https://pkg.go.dev/builtin#make
- The Go blog: Go Slices: usage and internals | https://go.dev/blog/slices-intro
---
## What it means

At {{file}}:{{line}}, `{{func}}` called `make` for a slice with a {{which}} that no slice can have. Usually it is negative. It can also be so large that it does not fit in memory at all.

## Why Go does this

The spec says that a negative length or capacity at run time, or a length larger than the capacity, causes a run-time panic. A slice cannot have -3 elements, and Go does not guess what you meant. A constant negative size does not even compile, so this panic always comes from a value computed while the program runs.

## How to find it

Print the size expression right before the `make`, and each value in it. Then ask how it can be negative:

- A subtraction such as `n - k` with `k > n`: an empty input, a window larger than the data, an offset past the end.
- A length from input that was converted from a larger or unsigned type and wrapped around.
- `make([]T, n, c)` with `c` smaller than `n`: the message then says `cap`.

Decide what the function should return in that case, often an empty result or an error, and check it before the `make`.

## Example

```go
func Gaps(points []float64, skip int) []float64 {
	gaps := make([]float64, len(points)-skip-1) // negative when skip >= len(points)
	// ...
}
```

Handle the small inputs first:

```go
	if skip >= len(points)-1 {
		return nil
	}
	gaps := make([]float64, len(points)-skip-1)
```

## Learn more
