---
id: test-example-mismatch
source: test
match: (?s)^got:\n.*\nwant:\n.*$
title: {{test}} printed something other than its Output comment
links:
- The Go blog: Testable Examples in Go | https://go.dev/blog/examples#output-comments
- Package testing: Examples | https://pkg.go.dev/testing#hdr-Examples
- Package fmt: printing verbs | https://pkg.go.dev/fmt#hdr-Printing
---
## What it means

`{{test}}` is an Example function. `go test` ran it, collected what it printed to standard output, and compared that with its `// Output:` comment. They differ. The block under `got:` is what the code printed; the block under `want:` is the comment.

## Why Go does this

Examples are documentation that is checked: pkg.go.dev shows them, and `go test` runs every Example that has an output comment. The comparison ignores space at the start and the end of the whole output, but every other character counts, line by line. That makes an Example a precise test of formatting.

## How to find it

Compare got and want line by line, and find the first line that differs. Then ask which call printed it:

- A value with different formatting: `fmt.Println` of a slice prints `[a b]`, `%q` adds quotes, `1.0/3` prints as `0.3333333333333333`.
- A missing or extra line: a loop that runs 1 time too few or too many, or an early return.
- A line you did not expect at all: a debug print that you left in your code also lands in got.
- Lines in a different order: a map range or goroutines. `fmt` prints maps sorted by key, but a `range` over a map has no fixed order.

Run only this Example: open the Steps tab and press the Run button of its step. Change your code, not the comment: the comment is the spec.

## Example

```text
got:
HELLO WORLD
want:
Hello World
```

The first line already differs: the function upper-cased every letter, not only the first of each word.

## Learn more
