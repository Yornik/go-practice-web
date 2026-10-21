---
id: vet-slog-key
source: vet
analyzer: slog
match: ^(?P<call>\S+) arg "(?P<arg>[^"]*)" should (?:probably )?be a string or a slog\.Attr \((?P<reason>.+)\)$
title: {{call}} expects a key where it finds the value {{arg}}
course: 41-middleware
links:
- Package log/slog | https://pkg.go.dev/log/slog
- Package log/slog: Logger.Log | https://pkg.go.dev/log/slog#Logger.Log
- The slog check of go vet | https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/slog
---
## What it means

After the message, `{{call}}` reads its arguments as pairs: a string key, then its value. At the place of a key it finds `{{arg}}`, which is neither a string nor a `slog.Attr`. slog does not fail. It logs the value under the key `!BADKEY`, and the pairs after it shift by 1, so a record such as `!BADKEY=200 !BADKEY=/x` ends up in your log.

## Why Go does this

The slog functions take their attributes as `...any`, so the compiler cannot check the pairs. The docs of `Logger.Log` say how slog reads them: an Attr is used as it is, a string that is not the last argument becomes the key of the next argument, and anything else becomes a value with the key `!BADKEY`. `go vet` checks the pairs, and `go test` runs this check before the tests.

## How to find it

Go to {{file}}:{{line}} and count the arguments after the message in pairs: key, value, key, value. Find the first pair that starts with a value, and add its key. To make a shift impossible, write each attribute with a function such as `slog.Int("status", status)` or `slog.String("path", path)`.

## Example

```go
slog.Info("request", status, path) // values without keys
```

Give every value a key:

```go
slog.Info("request", "status", status, "path", path)
```

## Learn more

The course exercise below practises structured logging with log/slog.
