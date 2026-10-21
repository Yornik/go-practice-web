---
id: vet-slog-missing-value
source: vet
analyzer: slog
match: ^call to (?P<call>\S+) (?:missing a final value|has a missing or misplaced value)$
title: A key passed to {{call}} has no value
course: 41-middleware
links:
- Package log/slog | https://pkg.go.dev/log/slog
- Package log/slog: Logger.Log | https://pkg.go.dev/log/slog#Logger.Log
- The slog check of go vet | https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/slog
---
## What it means

After the message, `{{call}}` reads its arguments as key-value pairs. The count does not add up: a key has no value after it, or a value is missing earlier in the list. slog logs a string without a value as a value under the key `!BADKEY`. Your record then reads `path=/x !BADKEY=status` instead of `status=200`.

## Why Go does this

The slog functions take their attributes as `...any`, so the compiler cannot check the pairs. The docs of `Logger.Log` say that a string becomes a key only when it is not the last argument, and that anything else becomes a value with the key `!BADKEY`. `go vet` counts the pairs, and `go test` runs this check before the tests.

## How to find it

Go to {{file}}:{{line}} and count the arguments after the message: the count must be even, unless some of them are `slog.Attr` values. Find the key without a value and add the value. Writing each attribute with a function such as `slog.Int("status", status)` keeps key and value together, so you cannot lose one of them.

## Example

```go
slog.Info("request", "path", path, "status") // "status" has no value
```

Add the value, or use an Attr:

```go
slog.Info("request", "path", path, slog.Int("status", status))
```

## Learn more

The course exercise below practises structured logging with log/slog.
