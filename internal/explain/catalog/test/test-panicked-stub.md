---
id: test-panicked-stub
source: test
match: (?s)^(?:[^\n]*\s)?panicked\b[^\n]*?"?TODO: implement\b.*$
match: (?s)^[^\n]*\bpanic: "?TODO: implement\b.*$
title: Your code reached a function that is still the course stub
links:
- The Go blog: Defer, Panic, and Recover | https://go.dev/blog/defer-panic-and-recover
- Package testing | https://pkg.go.dev/testing
- go test flags such as -run | https://pkg.go.dev/cmd/go#hdr-Testing_flags
---
## What it means

At {{file}}:{{line}} the test reports that your code panicked with `TODO: implement`: a function on the way is still the body the course ships with, `panic("TODO: implement")`. Nothing else is wrong. This is where the exercise, or this step of it, starts.

## Why Go does this

The stubs panic so that the package compiles before you have written anything. Many course tests call your code through a helper that recovers a panic and reports it, as in `Sum panicked: <value>`. That way 1 test fails, and the other tests still run. The message names the call, not the function that panicked. A message without a call, such as `slo.go:76: panicked: ...`, points at the stub line itself.

## How to find it

The function that panicked is the call that the message names, or a function it calls. A constructor such as `New` is often the first. Search for `TODO: implement` to see every stub that is left. Then work in small steps:

1. Read the doc comment above the function. Together with the README it is the spec.
2. Pick the step for this function in the Steps tab, or run `go test -run` with the filter of that step, so that only its tests run.
3. Replace the panic with code that compiles and returns something, even a zero value. The test then fails with a got and want message, and that message shows what to build next.

## Example

A generic stub, not one from this exercise:

```go
// Sum returns the sum of xs.
func Sum(xs []int) int {
	panic("TODO: implement")
}
```

Return a zero value first. Now the test reaches its checks:

```go
func Sum(xs []int) int {
	return 0
}
```

## Learn more
