---
id: runtime-deadlock
source: runtime
match: ^fatal error: all goroutines are asleep - deadlock!$
title: Every goroutine is waiting: a deadlock in {{func}}
course: 37-workerpool
links:
- The Go spec: Channel types | https://go.dev/ref/spec#Channel_types
- Effective Go: Channels | https://go.dev/doc/effective_go#channels
- Package sync: Mutex.Lock | https://pkg.go.dev/sync#Mutex.Lock
---
## What it means

Every goroutine of the program waits for something that only another waiting goroutine could do. Nothing can ever run again, so the runtime stopped the program. The first goroutine of the exercise package in the list waits at {{file}}:{{line}}, in `{{func}}`.

## Why Go does this

A goroutine that waits on a channel, a lock or a `WaitGroup` sleeps until another goroutine wakes it. When all of them sleep, the runtime knows that none will. It cannot see a partial deadlock, where some goroutines are stuck while others still run; then the program hangs instead. In a test binary the timer of `-timeout` counts as something that can still happen, so `go test` shows this message only with `-timeout=0`. Otherwise you get a test timeout.

## How to find it

Go prints each goroutine with what it waits for in brackets: `[chan send]`, `[chan receive]`, `[select]`, `[sync.Mutex.Lock]`, `[sync.WaitGroup.Wait]`. `(nil chan)` after it means the channel was never made. Skip goroutine 1, the test runner. For each goroutine in the exercise package, read its line and ask who should wake it:

- A send on an unbuffered channel needs a receiver that is ready at the same time. Is the receiver the same goroutine, further down?
- A `Lock` while the same goroutine already holds the lock, for example a method that calls another method that locks too.
- A `wg.Wait()` whose counter never reaches 0: a missing `Done` on some path.

## Example

```go
ch := make(chan int)
for _, x := range xs {
	ch <- 2 * x // no other goroutine receives: blocked forever
}
```

Send from another goroutine, and receive here:

```go
go func() {
	defer close(ch)
	for _, x := range xs {
		ch <- 2 * x
	}
}()
for d := range ch {
	out = append(out, d)
}
```

## Learn more
