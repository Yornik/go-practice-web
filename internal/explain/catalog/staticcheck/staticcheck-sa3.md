---
id: staticcheck-sa3
source: staticcheck
code: SA3
title: staticcheck found a problem in a test or benchmark ({{code}})
links:
- staticcheck: SA3, testing issues | https://staticcheck.dev/docs/checks/#SA3
- Package testing | https://pkg.go.dev/testing
---
## What it means

The test or benchmark at {{file}}:{{line}} uses package testing in a way that hides failures or distorts its results. The message names the problem. staticcheck groups these checks under SA3: testing issues. This is check {{code}}.

## Why Go does this

Package testing has rules that the compiler cannot check. The framework chooses `b.N` for a benchmark, so a benchmark that sets it measures the wrong thing. A test must report its failures in a way that go test sees. staticcheck knows these rules.

## How to find it

Read the message and look up the method or field that it names in the docs of package testing. Then check what the test really measures or verifies. The course ships its test files with each exercise, so you see this message in tests or benchmarks that you add yourself. The staticcheck page explains every check: search it for {{code}}.

## Example

```go
func BenchmarkSum(b *testing.B) {
	b.N = 1000 // the framework chooses b.N
	for i := 0; i < b.N; i++ {
		Sum(data)
	}
}
```

Let the framework choose the number of iterations:

```go
func BenchmarkSum(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Sum(data)
	}
}
```

## Learn more
