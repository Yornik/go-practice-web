---
id: vet-printf-wrap-directive
source: vet
analyzer: printf
match: ^(?P<call>\S+) does not support error-wrapping directive %w$
title: Only fmt.Errorf understands %w, and {{call}} does not
links:
- Package fmt: Errorf | https://pkg.go.dev/fmt#Errorf
- Package errors | https://pkg.go.dev/errors
- The Go blog: Working with Errors in Go 1.13 | https://go.dev/blog/go1.13-errors
---
## What it means

The verb `%w` wraps an error, and only `fmt.Errorf` supports it. `{{call}}` does not read `%w`, so it prints a marker such as `%!w(*fs.PathError=&{open x.txt 2})` instead of the error message.

## Why Go does this

`%w` does more than print. In `fmt.Errorf` it makes the new error keep the wrapped error, and gives it an `Unwrap` method that returns it. `errors.Is` and `errors.As` follow that chain to find the cause. A function that prints or returns a string has no error to keep the cause in, so `%w` means nothing there. The compiler cannot see this inside a format string. `go vet` can, and `go test` runs this check before the tests.

## How to find it

Go to {{file}}:{{line}} and decide what the call should produce:

- A new error that callers can inspect: use `fmt.Errorf` with `%w`, and return the result.
- A log line, a test message or a string: use `%v`. It prints the error message.

A test helper such as `t.Errorf` or `t.Fatalf` also needs `%v`.

## Example

```go
log.Printf("load %s: %w", path, err) // log cannot wrap
```

Print with %v, or wrap with fmt.Errorf when you return the error:

```go
log.Printf("load %s: %v", path, err)
return fmt.Errorf("load %s: %w", path, err)
```

## Learn more
