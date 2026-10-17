---
id: vet-printf-wrong-type
source: vet
analyzer: printf
match: ^(?P<call>\S+) format (?P<verb>%\S+) has arg (?P<arg>.+) of wrong type (?P<type>.+)$
title: The verb {{verb}} in {{call}} does not fit a {{type}}
links:
- Package fmt: Printing | https://pkg.go.dev/fmt#hdr-Printing
- The printf check of go vet | https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/printf
- Command vet | https://pkg.go.dev/cmd/vet
---
## What it means

`{{call}}` builds its output from a format string. The verb `{{verb}}` in that string expects one kind of value, but its argument `{{arg}}` is a `{{type}}`. Go still runs the call: fmt prints a marker such as `%!d(string=hello)` instead of the value. A test that compares the output then fails, and the reason is hard to see.

## Why Go does this

Printf-style functions take their arguments as `...any`, so the compiler accepts any value and cannot check them against the format. `go vet` reads the format string and checks every verb against the type of its argument. `go test` runs this check before it runs the tests, so a printf mistake stops the whole test run.

## How to find it

The column points at the verb inside the format string; the argument is the one at the same position in the argument list. Then decide which side is wrong:

- The verb. `%d` is for integers, `%s` and `%q` for strings, `%f` and `%g` for floats, `%t` for booleans, and `%v` prints any value. `%w` in `fmt.Errorf` needs an `error`.
- The argument. Maybe you passed the wrong variable, or the arguments are in the wrong order.

When you are not sure what a value is, print it with `%T`: it shows the type.

## Example

```go
fmt.Printf("%d requests from %s\n", name, count) // the arguments are swapped
```

Keep the arguments in the order of the verbs:

```go
fmt.Printf("%d requests from %s\n", count, name)
```

## Learn more
