---
id: runtime-exit-log-fatal
source: runtime
match: ^exit: \d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}(?:\.\d+)? (?P<message>(?s:.+))$
title: log.Fatal ended the test binary: {{message}}
course: 06-envconfig
links:
- Package log: Fatal | https://pkg.go.dev/log#Fatal
- Package os: Exit | https://pkg.go.dev/os#Exit
- Effective Go: Errors | https://go.dev/doc/effective_go#errors
---
## What it means

The test binary stopped after it printed `{{message}}`. The line starts with the date and time that the `log` package writes, so the code most likely called `log.Fatal`, `log.Fatalf` or `log.Fatalln`. Each prints the message and then calls `os.Exit(1)`. The running test never reported PASS or FAIL, the tests after it never ran, and `go test` only printed `FAIL` for the package.

## Why Go does this

`os.Exit` ends the process at once. Deferred functions and `t.Cleanup` functions do not run, so a buffered writer loses its last lines. The testing package never gets control back to report the test. So library code returns the error, and only `main` decides whether the program stops: `log.Fatal` belongs there. `log.Panic` is different: it panics, so deferred functions run and the test fails with a stack trace.

## How to find it

Search your files for the `log.Fatal` call that prints `{{message}}`. Start with the code that the running test calls. Without a test, the call ran in an `init` function or a package-level variable, before the first test. Replace the call with a `return` of the error, and add an `error` result if the function has none. When your files have no such call, the test made it: an Example that stops on an error it did not expect. Then `{{message}}` is an error your code returned. Ask why.

## Example

```go
func Hosts(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatal(err) // ends the test binary, and skips every defer
	}
	return strings.Fields(string(data))
}
```

Return the error, and let the caller decide what to do with it:

```go
func Hosts(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read hosts: %w", err)
	}
	return strings.Fields(string(data)), nil
}
```

## Learn more

In the exercise, the library returns every problem in 1 error, and only `main` calls `log.Fatal`.
