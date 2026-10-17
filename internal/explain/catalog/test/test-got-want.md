---
id: test-got-want
source: test
match: ^(?P<call>[^\n]+?) = (?P<got>[^\n]*?), want (?P<want>[^\n]+)(?:\n(?s:.*))?$
match: (?s)^(?P<call>[^\n]+)\ngot:?[ \t]+(?P<got>.+?)\nwant:?[ \t]+(?P<want>.+)$
fallback: true
title: {{call}} returned {{got}}, the test wants {{want}}
links:
- Package testing | https://pkg.go.dev/testing
- Go wiki: Table-driven tests | https://go.dev/wiki/TableDrivenTests
- Package fmt: printing verbs | https://pkg.go.dev/fmt#hdr-Printing
---
## What it means

The test called `{{call}}`. Your code returned `{{got}}`, and the test expected `{{want}}`. The test compared the 2 values at {{file}}:{{line}}; the lines above it show the input. When `{{test}}` has a slash, the name after it is the case: a row of a table-driven test, or in a fuzz test `seed#N`, the input of the `f.Add` call number N counted from 0, or a file in `testdata/fuzz` that holds an input that failed before.

## Why Go does this

A Go test is plain code: it calls your function, compares the result with `==`, `slices.Equal` or `reflect.DeepEqual`, and reports both values with `t.Errorf`. Your code ran without a crash; it computed a different answer.

## How to find it

1. Work this case by hand from the README: what should `{{call}}` return, and why?
2. Compare got and want piece by piece: an off-by-one, a missing first or last element, a wrong order, a rounding difference.
3. When the 2 look equal, the difference is invisible in this format: `%v` prints a nil slice and an empty slice both as `[]`, and it does not show spaces at the end of a string.
4. Print the intermediate values in your function with `fmt.Printf` and run only this test, with the step filter or `-run`.

Fix the cause, not the case: an `if` for this exact input makes the next case fail.

## Example

```text
x_test.go:17: Average([1 3]) = 1.3333333333333333, want 2
```

By hand: (1 + 3) / 2 = 2. The result is 4 / 3, so the code divides by 1 too many:

```go
return sum / float64(len(xs)+1) // should be len(xs)
```

## Learn more
