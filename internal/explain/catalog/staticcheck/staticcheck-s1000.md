---
id: staticcheck-s1000
source: staticcheck
code: S1000
title: A select with 1 case can be a plain channel operation
links:
- staticcheck: S1000 | https://staticcheck.dev/docs/checks/#S1000
- The Go spec: Select statements | https://go.dev/ref/spec#Select_statements
- The Go spec: For statements with range clause | https://go.dev/ref/spec#For_range
---
## What it means

This `select` has 1 case and no `default`. It waits for that 1 channel operation, which is exactly what a plain receive or send does. The code works, but the `select` suggests a choice that is not there. Inside a `for` loop, staticcheck suggests `for v := range ch` instead, which also stops when the channel is closed.

## Why Go does this

A `select` exists to wait on several channel operations at once, or to try 1 without blocking with `default`. With a single case it adds nothing. There is 1 difference to know for the loop form: a receive from a closed channel returns the zero value at once, so `for { select { case v := <-ch: ... } }` spins on a closed channel. `for v := range ch` ends instead.

## How to find it

Go to {{file}}:{{line}} and ask whether something else should be able to end the wait:

- Nothing: replace the `select` with the receive or send itself, or the loop with `range`.
- A timeout or a cancellation: add that case, such as `case <-ctx.Done():`. Then the `select` has a real choice to make.

## Example

```go
select {
case r := <-results:
	return r
}
```

Receive directly, or add the case that can end the wait:

```go
select {
case r := <-results:
	return r, nil
case <-ctx.Done():
	return 0, ctx.Err()
}
```

## Learn more
