---
id: runtime-exit-zero
source: runtime
match: ^panic: unexpected call to os\.Exit\(0\) during test$
title: {{func}} called os.Exit(0) while {{test}} ran
links:
- Package os: Exit | https://pkg.go.dev/os#Exit
- Go 1.16 release notes: go test | https://go.dev/doc/go1.16#go-test
- Effective Go: Errors | https://go.dev/doc/effective_go#errors
---
## What it means

`{{func}}` called `os.Exit(0)` while {{test}} ran. Outside a test, that ends the program with the code for success. In a test binary, `os.Exit(0)` panics with this message instead, and {{test}} fails with a stack trace.

## Why Go does this

`os.Exit` ends the process at once: deferred functions do not run, and the tests after this one never run. A test binary that exits with code 0 would look like a run in which every test passed. So `go test` runs the test binary with the flag `-test.paniconexit0`, and while the tests run, `os.Exit(0)` panics. This rule exists since Go 1.16. The panic runs the deferred functions on its way up, and the testing package reports the failure. An exit with another code is not caught: the binary stops without any result. Library code should not exit at all. It returns, with an error when something went wrong, and its caller decides what happens next.

## How to find it

In the stack trace under the panic, the frame `os.Exit(0x0)` is the call itself. The frame below it, `{{func}}` at {{file}}:{{line}}, is your code that called it. Ask what that path means. "Done" or "nothing to do" is a `return`. A problem is a returned error.

## Example

```go
func Hosts(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Println("no hosts file, nothing to do")
		os.Exit(0) // stops the caller too, and every test after it
	}
	return strings.Fields(string(data))
}
```

Return to the caller instead, and let it decide:

```go
func Hosts(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil // no file: nothing to do, and no error
	}
	if err != nil {
		return nil, fmt.Errorf("read hosts: %w", err)
	}
	return strings.Fields(string(data)), nil
}
```

## Learn more
