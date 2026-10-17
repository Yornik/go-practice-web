---
id: staticcheck-sa2
source: staticcheck
code: SA2
title: staticcheck found a concurrency problem ({{code}})
links:
- staticcheck: SA2, concurrency issues | https://staticcheck.dev/docs/checks/#SA2
- The Go Memory Model | https://go.dev/ref/mem
- Data Race Detector | https://go.dev/doc/articles/race_detector
---
## What it means

The code at {{file}}:{{line}} uses goroutines, a mutex or a WaitGroup in a way that is almost always a bug. Bugs of this kind show up only in some runs: a deadlock, a lost update or a data race. The message says what staticcheck saw. staticcheck groups these checks under SA2: concurrency issues. This is check {{code}}.

## Why Go does this

The compiler checks types, not the order in which goroutines run. staticcheck knows patterns that break synchronization, such as a deferred `Lock` right after a `Lock`. The race detector finds other problems while the tests run, so run them with `-race` as well.

## How to find it

Go to {{file}}:{{line}} and find the mutex, channel or WaitGroup that the message names. For each goroutine that uses it, ask: what does it wait for, and who releases it? Write the order of events down for 2 goroutines that run at the same time. The staticcheck page explains every check with an example: search it for {{code}}.

## Example

```go
mu.Lock()
defer mu.Lock() // at return, Lock waits for itself: the function never returns
update()
```

Defer the call that releases the lock:

```go
mu.Lock()
defer mu.Unlock()
update()
```

## Learn more
