---
id: vet-printf-too-few-args
source: vet
analyzer: printf
match: ^(?P<call>\S+) format (?P<verb>%\S*) reads arg #(?P<n>\d+), but call has (?P<have>\d+) args?$
title: The format of {{call}} has more verbs than arguments
links:
- Package fmt: Printing | https://pkg.go.dev/fmt#hdr-Printing
- The printf check of go vet | https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/printf
---
## What it means

The format string of `{{call}}` has a verb, `{{verb}}`, that needs argument number {{n}}. The call passes only {{have}} after the format string. Go still runs the call, and fmt prints a marker such as `%!d(MISSING)` where the value should be. A test that compares the output then fails on a strange string.

## Why Go does this

Printf-style functions take their arguments as `...any`, so the compiler cannot count them against the format. `go vet` parses the format string, counts the verbs and compares the count with the arguments. `go test` runs this check before the tests, so the mistake stops the test run.

## How to find it

The column points at the verb that has no argument. Count from the left: each verb such as `%d`, `%s` or `%v` takes the next argument. `%%` takes none, because it prints a percent sign. A `*` for the width, as in `%*d`, takes an argument of its own. Then decide which side is wrong:

- You forgot an argument, or you passed it to another call.
- The format has a verb too many. Often it is a `%` that you meant as a literal percent sign: write it as `%%`.

## Example

```go
fmt.Printf("%s: %d of %d done\n", name, done) // 3 verbs, 2 arguments
```

Pass 1 argument for every verb:

```go
fmt.Printf("%s: %d of %d done\n", name, done, total)
```

## Learn more
