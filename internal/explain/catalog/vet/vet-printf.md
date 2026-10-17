---
id: vet-printf
source: vet
analyzer: printf
title: go vet found a mistake in a printf-style call
links:
- Package fmt: Printing | https://pkg.go.dev/fmt#hdr-Printing
- The printf check of go vet | https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/printf
- Command vet | https://pkg.go.dev/cmd/vet
---
## What it means

The printf check of `go vet` found a call at {{file}}:{{line}} whose format string and arguments do not fit together. The message names the call, the verb and the argument that are wrong. Go would still run the call, and fmt would print a marker such as `%!i(int=3)` into the output instead of the value.

## Why Go does this

Printf-style functions take their arguments as `...any`, so the compiler cannot check them against the format. `go vet` parses the format string of every call to `fmt.Printf`, `fmt.Sprintf`, `fmt.Errorf`, `log.Printf`, `t.Errorf` and your own functions that pass a format on to them. `go test` runs this check before the tests, so the mistake stops the test run.

## How to find it

Read the message from left to right: the call, then the verb, then what is wrong with it. Look the verb up in the docs of package fmt. A verb is `%`, then optional flags, width and precision, then 1 letter. The verbs you need most:

- `%v` any value, `%+v` a struct with its field names, `%T` the type of a value.
- `%d` integers, `%s` and `%q` strings, `%g` and `%f` floats, `%t` booleans.
- `%%` a literal percent sign.

## Example

```go
return fmt.Sprintf("%i requests", n) // Go has no %i
```

Use the verb for integers:

```go
return fmt.Sprintf("%d requests", n)
```

## Learn more
