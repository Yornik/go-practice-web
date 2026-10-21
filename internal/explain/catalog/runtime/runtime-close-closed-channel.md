---
id: runtime-close-closed-channel
source: runtime
match: ^panic: close of closed channel$
title: {{func}} closed a channel that was already closed
course: 19-fanout
links:
- The Go spec: Close | https://go.dev/ref/spec#Close
- The Go blog: Pipelines and cancellation | https://go.dev/blog/pipelines#fan-out-fan-in
- Effective Go: Channels | https://go.dev/doc/effective_go#channels
---
## What it means

At {{file}}:{{line}}, `{{func}}` called `close` on a channel that something had closed before. The first close can be in another goroutine or another function; only the second one is in the stack trace.

## Why Go does this

`close(ch)` tells the receivers that no more values will come. That can happen only once. The spec says that closing a closed channel causes a run-time panic. Go has no function that asks "is this channel closed?" without receiving from it, and such an answer could be outdated the moment you act on it. The code must know who closes each channel, and when.

The usual rule: the sender closes, never a receiver. With several senders, 1 coordinator closes after all of them have finished.

## How to find it

Search for every `close(` of this channel, by its variable or field name. For each one, ask which goroutine runs it and how often:

- Once in each worker goroutine? Then the second worker panics.
- In a method such as `Stop` or `Close` that a caller can call twice?
- In a deferred call and on an explicit path as well?

Choose 1 owner that closes the channel exactly once. With several senders, close after `wg.Wait()`. If a method that closes may run more than once, record that it already ran, for example with `sync.Once`, and close only the first time.

## Example

```go
for _, job := range jobs {
	wg.Add(1)
	go func() {
		defer wg.Done()
		results <- run(job)
		close(results) // every worker closes: the second one panics
	}()
}
```

Let 1 goroutine close, after all senders are done:

```go
go func() {
	wg.Wait()
	close(results)
}()
```

## Learn more

The course exercise below closes a results channel once, after the last send.
