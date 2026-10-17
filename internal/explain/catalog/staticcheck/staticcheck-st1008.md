---
id: staticcheck-st1008
source: staticcheck
code: ST1008
title: Return the error as the last result
links:
- staticcheck: ST1008 | https://staticcheck.dev/docs/checks/#ST1008
- Effective Go: Multiple return values | https://go.dev/doc/effective_go#multiple-returns
- Effective Go: Errors | https://go.dev/doc/effective_go#errors
---
## What it means

This function returns an `error` before another result. In Go, the error is the last result by convention, as in `(int, error)`. Every caller expects that order, so a different one confuses readers and invites mistakes in the `if err != nil` checks.

## Why Go does this

Effective Go shows functions such as `func (f *File) Write(b []byte) (n int, err error)`: the values first, the error last. The whole standard library follows that order, so Go programmers read it without thinking. The compiler accepts any order. staticcheck reports an error in another place.

## How to find it

Go to {{file}}:{{line}} and move `error` to the end of the result list. Then update every `return` in the function and every call site, which the compiler will point out for you. When a function returns several values and an error, keep the error last as well.

## Example

```go
func ParseCount(s string) (error, int) {
	n, err := strconv.Atoi(s)
	return err, n
}
```

Put the error last:

```go
func ParseCount(s string) (int, error) {
	return strconv.Atoi(s)
}
```

## Learn more
