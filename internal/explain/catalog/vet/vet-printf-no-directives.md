---
id: vet-printf-no-directives
source: vet
analyzer: printf
match: ^(?P<call>\S+) call has arguments but no formatting directives$
title: {{call}} gets arguments, but its format string has no verbs
links:
- Package fmt: Printing | https://pkg.go.dev/fmt#hdr-Printing
- Package fmt: Errorf | https://pkg.go.dev/fmt#Errorf
- The Go blog: Working with Errors in Go 1.13 | https://go.dev/blog/go1.13-errors
---
## What it means

`{{call}}` reads its first argument as a format string, and that string has no verb such as `%v`. The other arguments have no place in the output, so fmt appends them with a marker. `fmt.Errorf("parse port", err)` returns an error with the text `parse port%!(EXTRA *errors.errorString=...)`. The error is also not wrapped, so `errors.Is` cannot find the cause.

## Why Go does this

Only the functions whose names end in `f` read a format string. They take their arguments as `...any`, so the compiler cannot check them. `go vet` parses the format string and checks that every argument has a verb. `go test` runs this check before the tests, so the mistake stops the test run.

## How to find it

Go to {{file}}:{{line}} and decide what each extra argument is for:

- A value in the message: add a verb for it. `%v` prints any value.
- An error that callers must be able to inspect: in `fmt.Errorf`, use `%w`. It wraps the error, so `errors.Is` and `errors.As` find it.
- Values that only need spaces between them: use a function without `f`, such as `fmt.Println` or `t.Log`.

## Example

```go
return 0, fmt.Errorf("parse port", err) // err has no verb
```

Wrap the cause with `%w`:

```go
return 0, fmt.Errorf("parse port %q: %w", s, err)
```

## Learn more
