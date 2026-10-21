---
id: staticcheck-sa1032
source: staticcheck
code: SA1032
title: The arguments of errors.Is are swapped
course: 26-logfmt
links:
- staticcheck: SA1032 | https://staticcheck.dev/docs/checks/#SA1032
- Package errors: Is | https://pkg.go.dev/errors#Is
- The Go blog: Working with Errors in Go 1.13 | https://go.dev/blog/go1.13-errors
---
## What it means

`errors.Is(err, target)` asks: is `err`, or any error that it wraps, the error `target`? Here the first argument is a package-level error such as `io.EOF`, and the second is the error you got. The call asks the reverse question. It works while your error is exactly `io.EOF`, and fails as soon as someone wraps it with `fmt.Errorf("...: %w", err)`.

## Why Go does this

The docs of `errors.Is` say that it searches the tree of its first argument: the error itself and every error that `Unwrap` returns. A sentinel such as `io.EOF` wraps nothing, so its tree holds only itself. Both arguments have type `error`, so the compiler cannot see the mistake. staticcheck reports a package-level error variable in the first place.

## How to find it

Go to {{file}}:{{line}} and swap the arguments: the error that you got first, the error that you look for second. Read it as a question: is err an EOF? Then test it with a wrapped error, as callers will produce one.

## Example

```go
if errors.Is(io.EOF, err) { // searches inside io.EOF
```

Put the error that you got first:

```go
if errors.Is(err, io.EOF) {
```

## Learn more

The course exercise below practises errors.Is with wrapped errors.
