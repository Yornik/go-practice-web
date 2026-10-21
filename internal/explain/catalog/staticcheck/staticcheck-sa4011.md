---
id: staticcheck-sa4011
source: staticcheck
code: SA4011
title: This break leaves the select or switch, not the loop around it
course: 37-workerpool
links:
- staticcheck: SA4011 | https://staticcheck.dev/docs/checks/#SA4011
- The Go spec: Break statements | https://go.dev/ref/spec#Break_statements
- The Go spec: Select statements | https://go.dev/ref/spec#Select_statements
---
## What it means

In Go, `break` ends the innermost `for`, `switch` or `select`. Your `break` is the last statement of a case, so it ends the `select` or `switch`, which was about to end anyway. The loop around it goes on. You most likely meant to leave the loop, and now it never stops.

## Why Go does this

The Go spec says that `break` terminates the innermost `for`, `switch` or `select` statement in the same function. A case in Go ends on its own, without a `break`, so a `break` at its end does nothing. Code from C or Java often carries this habit. The compiler accepts it, and staticcheck reports it.

## How to find it

Go to {{file}}:{{line}} and decide how the loop should end:

- With a labeled break: put a label before the loop, such as `loop:`, and write `break loop`.
- With a `return`, when nothing after the loop needs to run.

Check the other `break` statements inside the same `select` or `switch` too: each of them also leaves only the `select` or `switch`.

## Example

```go
for {
	select {
	case e := <-events:
		handle(e)
	case <-done:
		break // leaves the select, not the for
	}
}
```

Return, or break out of the labeled loop:

```go
	case <-done:
		return
```

## Learn more

The course exercise below practises loops that select on channels and stop on time.
