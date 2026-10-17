---
id: staticcheck-s1028
source: staticcheck
code: S1028
title: fmt.Errorf builds a formatted error in 1 step
links:
- staticcheck: S1028 | https://staticcheck.dev/docs/checks/#S1028
- Package fmt: Errorf | https://pkg.go.dev/fmt#Errorf
- Package errors: New | https://pkg.go.dev/errors#New
---
## What it means

`errors.New(fmt.Sprintf(...))` formats a string and then turns it into an error. `fmt.Errorf` does both in 1 call. It can also wrap another error with `%w`, so that `errors.Is` and `errors.As` find it. The combination of `errors.New` and `Sprintf` can never do that.

## Why Go does this

`errors.New` takes a finished string, and `fmt.Errorf` takes a format string with arguments and returns an error. The compiler accepts both forms. staticcheck suggests the shorter one, because it is the usual way to build an error with values in it.

## How to find it

Go to {{file}}:{{line}} and replace the 2 calls with `fmt.Errorf` and the same format and arguments. If 1 of the arguments is an error that callers may want to inspect, use `%w` for it instead of `%v`. Use `errors.New` alone for a fixed message without values, such as a sentinel error at package level.

## Example

```go
return errors.New(fmt.Sprintf("missing key %q", key))
```

Format the error in 1 call:

```go
return fmt.Errorf("missing key %q", key)
```

## Learn more
