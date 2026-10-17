---
id: staticcheck-sa2002
source: staticcheck
code: SA2002
match: ^the goroutine calls (?P<call>\S+), which must be called in the same goroutine as the test$
title: The goroutine calls {{call}}, which works only in the test's own goroutine
links:
- staticcheck: SA2002 | https://staticcheck.dev/docs/checks/#SA2002
- Package testing: T.FailNow | https://pkg.go.dev/testing#T.FailNow
---
## What it means

A goroutine that the test started calls `{{call}}`. `Fatal`, `FailNow` and the `Skip` methods stop the goroutine that calls them, not the test. The test function goes on as if nothing happened. It may pass, hang while it waits for the goroutine, or fail later with a confusing message.

## Why Go does this

`FailNow`, which `Fatal` calls, stops its goroutine with `runtime.Goexit`. The docs of `T.FailNow` say that it must be called from the goroutine that runs the test, not from goroutines that the test created. The compiler cannot see which goroutine makes a call. `go vet` reports the same mistake with its testinggoroutine check.

## How to find it

The message points at the `go` statement; the call is inside its function. Choose 1 fix:

- Report with `t.Error` or `t.Errorf` inside the goroutine, and return. These are safe from any goroutine.
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

Hand the error to the test goroutine:

```go
errc := make(chan error, 1)
go func() { errc <- Fetch() }()
if err := <-errc; err != nil {
	t.Fatal(err)
}
```

## Learn more
