---
id: staticcheck-sa5001
source: staticcheck
code: SA5001
match: ^should check error returned from (?P<call>.+) before deferring (?P<close>.+)$
title: {{close}} is deferred before the error of {{call}} is checked
links:
- staticcheck: SA5001 | https://staticcheck.dev/docs/checks/#SA5001
- Effective Go: Defer | https://go.dev/doc/effective_go#defer
- Package os: Open | https://pkg.go.dev/os#Open
---
## What it means

You defer `{{close}}` before you check the error that `{{call}}` returned. When the call fails, the value that you close is nil or invalid. The deferred call then runs on it when the function returns: depending on the type, it panics or returns an error that nobody reads. On the success path the code works, so tests that only open existing files never show it.

## Why Go does this

A function that returns a value and an error gives no usable value when the error is not nil. A `defer` statement evaluates its receiver and arguments at once, so it captures the nil value. The convention in Go is to check the error first, and to defer the cleanup only for a value that you really got.

## How to find it

Go to {{file}}:{{line}} and move the `defer` below the `if err != nil { ... }` block. The order is always the same: call, check the error, defer the cleanup, use the value. If closing can fail in a way that matters, such as for a file that you wrote, check the error of `Close` too instead of deferring it blindly.

## Example

```go
f, err := os.Open(path)
defer f.Close() // f is nil when Open fails
if err != nil {
	return nil, err
}
```

Check the error first, then defer the close:

```go
f, err := os.Open(path)
if err != nil {
	return nil, err
}
defer f.Close()
```

## Learn more
