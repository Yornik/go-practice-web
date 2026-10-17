---
id: vet-printf-too-many-args
source: vet
analyzer: printf
match: ^(?P<call>\S+) call needs (?P<want>\d+) args? but has (?P<have>\d+) args?$
title: {{call}} gets more arguments than its format has verbs
links:
- Package fmt: Printing | https://pkg.go.dev/fmt#hdr-Printing
- The printf check of go vet | https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/printf
---
## What it means

The format string of `{{call}}` has verbs for {{want}} arguments, but the call passes {{have}}. Go still runs the call. fmt adds every extra value at the end of the output with a marker, such as `%!(EXTRA string=api)`. The output then holds text that you did not plan, and a test that compares it fails.

## Why Go does this

Printf-style functions take their arguments as `...any`, so the compiler cannot count them against the format. `go vet` parses the format string and checks that every argument has a verb. `go test` runs this check before the tests, so the mistake stops the test run.

## How to find it

Go to {{file}}:{{line}} and pair the verbs with the arguments from the left. The first argument without a verb is the extra one. Then ask:

- Did you add a value but forget its verb in the format?
- Is the argument left over from an older version of the message?
- Does the format use `%%`? It prints a percent sign and takes no argument.

## Example

```go
log.Printf("retry %d after %v", attempt, delay, err) // 2 verbs, 3 arguments
```

Give the error its own verb:

```go
log.Printf("retry %d after %v: %v", attempt, delay, err)
```

## Learn more
