---
id: vet-testinggoroutine
source: vet
analyzer: testinggoroutine
title: A test calls Fatal or Skip from a goroutine that it started
links:
- Package testing: T.FailNow | https://pkg.go.dev/testing#T.FailNow
- The testinggoroutine check of go vet | https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/testinggoroutine
---
## What it means

At {{file}}:{{line}}, a goroutine that the test started calls `t.Fatal`, `t.Fatalf`, `t.FailNow` or 1 of the `Skip` methods. These methods stop the goroutine that calls them, not the test. The test function goes on as if nothing happened. It may pass, hang while it waits for a result, or fail later with a confusing message.

## Why Go does this

`FailNow`, which `Fatal` calls, stops its goroutine with `runtime.Goexit`. The docs of `T.FailNow` say that it must be called from the goroutine that runs the test, not from other goroutines that the test created. The compiler cannot see which goroutine calls a method, so `go vet` reports these calls inside a `go` statement.

## How to find it

Go to {{file}}:{{line}} and choose 1 fix:

- Report with `t.Error` or `t.Errorf` inside the goroutine and return. These are safe from any goroutine.
- Send the error over a channel to the test goroutine, and call `t.Fatal` there.

The course ships its test files with each exercise. You see this message in tests that you add yourself.

## Example

```go
go func() {
	if err := Fetch(); err != nil {
		t.Fatal(err) // stops this goroutine only
	}
}()
```

Report with t.Error, which works from any goroutine:

```go
go func() {
	if err := Fetch(); err != nil {
		t.Error(err)
	}
}()
```

## Learn more
