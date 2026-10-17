---
id: guide-failing-test
source: guide
title: How to read a failing test
links:
- Package testing | https://pkg.go.dev/testing
- go test flags such as -run and -v | https://pkg.go.dev/cmd/go#hdr-Testing_flags
- The Go blog: Testable Examples in Go | https://go.dev/blog/examples
---
## The name

`--- FAIL: TestSplit/two_fields (0.00s)` names the test that failed. After the slash comes the subtest, the case of a table-driven test; spaces in its name become underscores. The time in parentheses is how long it ran.

## The line

Under the name, each message starts with a file and a line, such as `split_test.go:42:`. That is where the test called `t.Errorf` or `t.Fatalf`. Open that line: the code just above it shows the input and how the test checks the result. `t.Errorf` lets the test go on and report more problems; `t.Fatalf` stops this test.

## Got and want

The course tests write their messages as the call, what your code returned, and what the test expected:

```text
Split("a,b") = [a,b], want [a b]
```

Compare the 2 values character by character. A difference that is hard to see, such as a space, a quote or a missing element, is often the bug. For an Example, the output under `got:` is what your code printed and `want:` is the `// Output:` comment of the example. Only leading and trailing space of the whole output is ignored.

## Run 1 test

Run only the failing test while you fix it: pick its step, or use `go test -run 'TestSplit/two_fields' -v`. `-run` takes a regular expression per level of the name, separated by a slash. `-v` also prints the logs of the tests that pass.

If the output says `panic: test timed out`, a test ran longer than the timeout allows. Usually it waits for something that never happens. The goroutine list below it shows where it waits.

## Learn more
