---
id: runtime-todo-implement
source: runtime
match: ^panic: TODO: implement$
title: {{func}} is still the course stub
links:
- The Go blog: Defer, Panic, and Recover | https://go.dev/blog/defer-panic-and-recover
- Package testing | https://pkg.go.dev/testing
- go test flags such as -run | https://pkg.go.dev/cmd/go#hdr-Testing_flags
---
## What it means

The test called `{{func}}` in {{file}}, and that function still has the body the course ships with: `panic("TODO: implement")`. Nothing else is wrong. This is where the exercise starts: `{{func}}` is yours to write.

## Why Go does this

A panic that no deferred function recovers ends the program. The testing package recovers it only to report this test as failed and print the stack, and then panics again. So the whole test binary stops here: the tests after this one do not run at all. You see 1 failure, even when more functions are still stubs.

## How to find it

In the stack trace under the panic, the first frame in the exercise package is `{{func}}` at {{file}}:{{line}}. The frame below it is the line in the test that called it. Then work in small steps:

1. Read the doc comment above `{{func}}`. Together with the spec, it says what to build.
2. Open the Steps tab and press the Run button of the step for this function, so that only its tests run.
3. Replace the panic with code that compiles and returns something, even a zero value. The tests then fail with got and want messages instead of a crash, and those messages show you what to build next.

## Example

A generic stub, not one from this exercise:

```go
// Sum returns the sum of xs.
func Sum(xs []int) int {
	panic("TODO: implement")
}
```

Return a zero value first. Now every test runs and tells you what it expects:

```go
func Sum(xs []int) int {
	return 0
}
```

## Learn more
