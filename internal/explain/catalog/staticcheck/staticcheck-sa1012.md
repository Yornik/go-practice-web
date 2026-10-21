---
id: staticcheck-sa1012
source: staticcheck
code: SA1012
title: A nil context is passed where a context.Context belongs
course: 21-rollout
links:
- staticcheck: SA1012 | https://staticcheck.dev/docs/checks/#SA1012
- Package context | https://pkg.go.dev/context
- The Go blog: Go Concurrency Patterns: Context | https://go.dev/blog/context
---
## What it means

You pass `nil` as a `context.Context`. Code that receives a context calls its methods, such as `Done`, `Err` or `Value`, and a nil context makes those calls panic. Some functions check first: `http.NewRequestWithContext` returns the error `net/http: nil Context`. Either way, the call cannot be canceled or given a deadline.

## Why Go does this

`context.Context` is an interface, and nil is a valid value for any interface, so the compiler accepts it. The docs of package context say: do not pass a nil Context, even if a function permits it, and pass `context.TODO` if you are unsure which Context to use.

## How to find it

Go to {{file}}:{{line}} and find the context that this code should use:

- A function that does I/O or waits takes `ctx context.Context` as its first parameter, and passes it on to the calls it makes. If the function has one, use it.
- At the top of a program or a test there is no parent yet: start with `context.Background()`.
- `context.TODO()` marks a place where you have not decided yet. Replace it before you finish.

## Example

```go
req, err := http.NewRequestWithContext(nil, http.MethodGet, url, nil)
```

Pass the context of the caller on:

```go
func Get(ctx context.Context, url string) (*http.Request, error) {
	return http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
}
```

## Learn more

The course exercise below practises a wait loop that watches its context.
