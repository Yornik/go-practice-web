---
id: test-goroutine-leak
source: test
match: ^\d+ goroutines?(?:\(s\))? (?:are )?still running (?P<wait>(?:\d+(?:\.\d+)?(?:ns|µs|us|ms|s|m|h))+) after (?P<call>[^\n]+?) returned(?s:.*)$
title: A goroutine started by {{call}} never ended
course: 19-fanout
links:
- The Go blog: Pipelines and cancellation | https://go.dev/blog/pipelines#explicit-cancellation
- Package runtime: NumGoroutine | https://pkg.go.dev/runtime#NumGoroutine
- Package context | https://pkg.go.dev/context
---
## What it means

After `{{call}}` returned, the test waited {{wait}} for the number of goroutines to drop back to what it was before the call. It did not: a goroutine that your code started is still alive, blocked on something. That is a goroutine leak: it keeps its memory, and in a long-running service the leaks add up.

## Why Go does this

Go cannot stop a goroutine from the outside; it ends when its function returns. So a function that starts goroutines must make sure they can finish, also when it returns early. A goroutine blocked on a send that nobody will receive, or on a receive from a channel that nobody sends on or closes, waits forever.

## How to find it

Find each `go` statement in the code that `{{call}}` runs. For each goroutine, follow every path to its end and ask: can it block here after `{{call}}` has returned? Look for:

- A send on an unbuffered channel whose reader stopped reading.
- A `range` over a channel that nobody closes.
- A wait for a signal that is not sent on the error path.

If the test printed all goroutines, the word in brackets, such as `[chan send]`, says where each of yours is stuck. Give every goroutine a way out: a `select` on a `done` channel or `ctx.Done()`, a buffer with room for every send, or a `close` on the channel it ranges over.

## Example

```go
found := make(chan string)
go func() {
	for _, l := range lines {
		if strings.Contains(l, word) {
			found <- l // blocks after the reader returned
		}
	}
	close(found)
}()
return <-found
```

Let the goroutine stop when the reader leaves:

```go
done := make(chan struct{})
defer close(done)
// in the goroutine:
select {
case found <- l:
case <-done:
	return
}
```

## Learn more

The course exercise below practises fan-out without goroutine leaks.
