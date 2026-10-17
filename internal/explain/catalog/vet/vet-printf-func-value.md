---
id: vet-printf-func-value
source: vet
analyzer: printf
match: ^(?P<call>\S+) format %\S+ arg (?P<arg>.+) is a func value, not called$
match: ^(?P<call>\S+) arg (?P<arg>.+) is a func value, not called$
title: {{arg}} is a function, and {{call}} prints the function, not its result
links:
- The Go spec: Function types | https://go.dev/ref/spec#Function_types
- The Go spec: Method values | https://go.dev/ref/spec#Method_values
- Package fmt: Printing | https://pkg.go.dev/fmt#hdr-Printing
---
## What it means

You pass `{{arg}}` to `{{call}}` without the parentheses of a call. Go passes the function itself, not its result, and fmt prints a marker or an address such as `0x533700`. You most likely meant to call it: `{{arg}}()`.

## Why Go does this

In Go a function is a value. You can store it in a variable, pass it to another function or return it. A method also becomes a value when you leave out the parentheses: `err.Error` is a function that you can call later. Printf-style functions take `...any`, so they accept a function like any other value. `go vet` reports it, because printing a function is almost never what you want.

## How to find it

Go to {{file}}:{{line}} and find `{{arg}}` in the arguments. Check whether it is a function or method name without `()`. The usual suspects:

- `err.Error` instead of `err.Error()`. With `%v` you can also pass `err` itself.
- `time.Now` instead of `time.Now()`.
- A method such as `x.String` or `x.Len` without its call.

## Example

```go
return fmt.Sprintf("request failed: %s", err.Error) // the method itself
```

Call the method, or pass the error and let fmt call it:

```go
return fmt.Sprintf("request failed: %v", err)
```

## Learn more
