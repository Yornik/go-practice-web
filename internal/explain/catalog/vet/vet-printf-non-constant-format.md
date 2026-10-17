---
id: vet-printf-non-constant-format
source: vet
analyzer: printf
match: ^non-constant format string in call to (?P<call>\S+)$
title: The format string of {{call}} is a variable, not a constant
links:
- Go 1.24 release notes: Vet | https://go.dev/doc/go1.24#vet
- Package fmt: Printing | https://pkg.go.dev/fmt#hdr-Printing
- Package errors: New | https://pkg.go.dev/errors#New
---
## What it means

`{{call}}` reads its first argument as a format string, and you passed a variable without other arguments. The text in that variable becomes the format. When it contains a `%`, fmt reads a verb: the message `100% done` prints as `100%!d(MISSING)one`. Text that comes from input, such as a log line, a path or a URL, can contain `%` at any time.

## Why Go does this

The compiler cannot know what a variable will hold. Since Go 1.24, `go vet` reports a variable format string without arguments, in modules whose `go.mod` says `go 1.24` or later. The course module does. `go test` runs this check before the tests, so the call stops the test run.

## How to find it

Go to {{file}}:{{line}} and ask what the variable holds:

- A finished message: use a function that reads no format. `errors.New(msg)` instead of `fmt.Errorf(msg)`, `fmt.Print` instead of `fmt.Printf`, `t.Error` instead of `t.Errorf`.
- A message that still needs values: write the format as a constant in the call and pass the values as arguments.
- Text that you must print as it is: `fmt.Printf("%s", msg)` works too.

## Example

```go
msg := "empty line in " + name
return fmt.Errorf(msg) // a % in name breaks the message
```

Let fmt insert the value, or use errors.New for a finished message:

```go
return fmt.Errorf("empty line in %s", name)
```

## Learn more
