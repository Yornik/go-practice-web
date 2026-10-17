---
id: vet-tests
source: vet
analyzer: tests
title: A test, example or fuzz function has a name or signature that go test cannot use
links:
- Package testing | https://pkg.go.dev/testing
- Command go: Test packages | https://pkg.go.dev/cmd/go#hdr-Test_packages
- The tests check of go vet | https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/tests
---
## What it means

A function in the test file at {{file}}:{{line}} looks like a test, an example or a fuzz target, but go test cannot use it as written. The message names the function and the problem. A test named `Testparse` is the classic case: go test does not run it at all, and nothing fails, so you think it passed.

## Why Go does this

go test finds tests by their name and signature. The docs of package testing require `func TestXxx(t *testing.T)`, where Xxx does not start with a lower-case letter. Examples follow `ExampleXxx`, and the name must refer to something that exists. A function that almost fits is ignored without a word. `go test` runs this check before the tests.

## How to find it

Read the message: it names the function. Then check the parts of the declaration:

- The name: `Test`, `Example`, `Benchmark` or `Fuzz`, then an upper-case letter, a digit or `_`.
- The parameters: exactly 1, of type `*testing.T`, `*testing.B` or `*testing.F`. Examples take none.
- The results and type parameters: none.

The course ships its test files with each exercise. You see this message in tests that you add yourself.

## Example

```go
func Testparse(t *testing.T) { // never runs: lower-case p
```

Start the rest of the name with an upper-case letter:

```go
func TestParse(t *testing.T) {
```

## Learn more
