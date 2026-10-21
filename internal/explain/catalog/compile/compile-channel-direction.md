---
id: compile-channel-direction
source: compile
match: ^invalid operation: cannot (?P<op>send to|close|receive from) (?:receive|send)-only channel (?:(?:<-)?chan(?:<-)? .+? )?(?P<expr>\S+) \(.+\)$
match: ^cannot range over (?P<expr>.+?) \(.+\): (?P<op>receive from) send-only channel .+$
title: The channel {{expr}} only works in 1 direction, and you tried to {{op}} it
course: 21-rollout
links:
- The Go spec: Channel types | https://go.dev/ref/spec#Channel_types
- The Go spec: Close | https://go.dev/ref/spec#Close
- The Go blog: Go Concurrency Patterns: Pipelines and cancellation | https://go.dev/blog/pipelines
---
## What it means

At {{file}}:{{line}} you tried to {{op}} the channel `{{expr}}`, but its type allows only the other direction. A `<-chan T` is receive-only: you can receive from it and range over it. A `chan<- T` is send-only: you can send to it and close it.

## Why Go does this

A direction in a channel type is a promise in the API. A function that takes `<-chan T` promises to only read; one that returns `<-chan T` keeps the right to send and to close for itself. Closing belongs to the sender: a receiver that closes a channel makes the next send panic. The compiler enforces these roles, so a whole class of concurrency bugs cannot happen.

## How to find it

Find out who owns the channel: who creates it, who sends on it and who closes it.

- The code that sends and closes should hold the bidirectional `chan T` it created, or a `chan<- T`.
- Code that only receives gets a `<-chan T`, and it never closes it. It stops when the channel is closed, or when a context is done.
- If a receiver must tell the sender to stop, use a separate signal, such as a `done` channel that the receiver closes, or a `context.Context`.

## Example

```go
func Drain(ch <-chan int) int {
	n := 0
	for range ch {
		n++
	}
	close(ch) // invalid operation: cannot close receive-only channel ch (variable of type <-chan int)
	return n
}
```

Leave the close to the sender; the loop ends when the sender closes the channel:

```go
func Drain(ch <-chan int) int {
	n := 0
	for range ch {
		n++
	}
	return n
}
```

## Learn more

The course exercise below takes a receive-only channel of ticks.
