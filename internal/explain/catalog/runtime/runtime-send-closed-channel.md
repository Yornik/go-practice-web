---
id: runtime-send-closed-channel
source: runtime
match: ^panic: send on closed channel$
title: {{func}} sent on a channel that was already closed
course: 19-fanout
links:
- The Go spec: Send statements | https://go.dev/ref/spec#Send_statements
- The Go blog: Pipelines and cancellation | https://go.dev/blog/pipelines#explicit-cancellation
- Data Race Detector: send and close | https://go.dev/doc/articles/race_detector#Unsynchronized_send_and_close_operations
---
## What it means

At {{file}}:{{line}}, `{{func}}` sent a value on a channel that another part of the code had closed. A close promises the receivers that no more values will come; this send broke that promise.

## Why Go does this

The spec says that a send on a closed channel causes a run-time panic. Receiving is still fine after a close: receivers get the values left in the buffer, then zero values with `ok == false`. Only the sending side is forbidden. That is why in Go the sender closes, never a receiver, and only when no send can follow.

## How to find it

Find the `close` of this channel and ask whether a send can still happen after it. 2 patterns cause most of these panics:

- A receiver closes the channel to say "stop", while senders still run. Use a separate signal for "stop": a `done` channel that only the receiver closes, or a `context.Context` that is cancelled, and let the senders `select` on it.
- A `Close` method closes the channel while another goroutine is inside a method that sends. Then the send and the close need the same lock, or 1 goroutine must own both.

With several senders, close only after all of them have returned, for example after `wg.Wait()`.

## Example

```go
results := make(chan int)
for _, u := range urls {
	go func() { results <- fetch(u) }()
}
first := <-results
close(results) // the other goroutines still send: panic
```

Do not close a channel that others still send on. Give it room for every send, so no sender blocks, and let it be collected:

```go
results := make(chan int, len(urls))
```

## Learn more

The course exercise below practises close rules when many goroutines send and receive.
