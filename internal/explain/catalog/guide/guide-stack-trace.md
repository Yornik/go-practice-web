---
id: guide-stack-trace
source: guide
title: How to read a stack trace
links:
- The Go blog: Defer, Panic, and Recover | https://go.dev/blog/defer-panic-and-recover
- The Go spec: Run-time panics | https://go.dev/ref/spec#Run_time_panics
- Package runtime: GOTRACEBACK | https://pkg.go.dev/runtime#hdr-Environment_Variables
---
## The first line

A crash starts with what went wrong: `panic: ` and the value that was passed to `panic`, or `fatal error: ` for a problem the program cannot recover from, such as a deadlock. In a test you may see `[recovered]` after it: the testing package caught the panic to report the failing test, then panicked again.

## Goroutines and frames

Then Go prints the goroutines, each with a header such as `goroutine 19 [running]:`. The word in brackets is the state: `running`, or what the goroutine waits for, such as `chan receive`, `sync.Mutex.Lock` or `sync.WaitGroup.Wait`.

Below each header come the frames, the most recent call first. Every frame has 2 lines:

```text
example.com/retry.Do({0x5b1e20, 0xc000012345}, 0x3)
	/work/retry.go:42 +0x1d
```

The first line is the function with its package path. The arguments are raw machine words in hex, often marked with `?` because they can be stale, and `(...)` means the call was inlined. Do not try to read your values from them. The second line is the file and the line that function was executing. The `+0x1d` offset is a position in the machine code; you can ignore it. A line `created by ... in goroutine 7` shows where this goroutine was started.

## Find your frame

Read down from the top and skip the frames in `runtime` and `testing`. The first frame in the exercise package is where it happened. The frame below it shows who called it, often a line in a test file. Open both lines.

For a deadlock or a test timeout, Go prints every goroutine. Look for the goroutines in the exercise package and for what each one waits on. 2 goroutines that wait for each other, or a receive that no send will ever match, is the usual cause.

## Learn more
