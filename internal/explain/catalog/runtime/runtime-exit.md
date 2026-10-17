---
id: runtime-exit
source: runtime
match: ^exit: (?P<output>(?s:.+))$
fallback: true
title: The test binary exited before the tests reported a result
course: 06-envconfig
links:
- Package os: Exit | https://pkg.go.dev/os#Exit
- Effective Go: Errors | https://go.dev/doc/effective_go#errors
- The Go blog: Error handling and Go | https://go.dev/blog/error-handling-and-go
---
## What it means

The test binary stopped without a panic. The last line of the output is `{{output}}`. No test reported PASS or FAIL after it, and the tests that came next never ran. `go test` only printed `FAIL` for the package, and `exit status` with the code before that when the binary printed nothing at all. Code that calls `os.Exit` with a code other than 0 does exactly this.

## Why Go does this

`os.Exit` ends the process at once with the code you pass. Deferred functions do not run, `t.Cleanup` functions do not run, and the testing package never gets control back to report the test. While the tests run, `os.Exit(0)` panics instead, so that an exit cannot pass for a successful run; any other code exits. Library code should return an error and let its caller decide. Only `main` knows whether the program should stop, and with which exit code.

## How to find it

Search your files for `os.Exit` and `log.Fatal`. The run shows this explanation under the test that was running: start with the code that this test calls, and look at the line it printed last. Without a test, the exit ran in an `init` function or in the initializer of a package-level variable, before the first test. Turn each exit into a returned error.

## Example

```go
func Port(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bad port:", s)
		os.Exit(1) // the caller never hears about it
	}
	return n
}
```

Return the error instead:

```go
func Port(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("bad port %q: %w", s, err)
	}
	return n, nil
}
```

## Learn more

In the exercise, the library reports every problem in 1 error, and only `main` decides to stop.
