---
id: staticcheck-sa2001
source: staticcheck
code: SA2001
title: The mutex is unlocked right after it is locked, so it protects nothing
course: 20-counters
links:
- staticcheck: SA2001 | https://staticcheck.dev/docs/checks/#SA2001
- Package sync: Mutex | https://pkg.go.dev/sync#Mutex
- The Go Memory Model | https://go.dev/ref/mem
---
## What it means

`Lock` is followed directly by `Unlock`, so the critical section between them is empty. The code that should run under the lock comes after the `Unlock` and runs without it. 2 goroutines can then change the shared state at the same time, and `-race` reports a data race. Most often you meant `defer mu.Unlock()`.

## Why Go does this

A mutex protects only the code between `Lock` and `Unlock`. The compiler does not know which data a mutex is meant to guard. staticcheck reports an empty critical section, because it is almost always a typo. In rare code it is a way to wait for another goroutine, and then it needs a comment that says so.

## How to find it

Go to {{file}}:{{line}} and list the lines that read or write the state that the mutex guards. All of them belong between `Lock` and `Unlock`. `defer mu.Unlock()` right after `Lock` covers the rest of the function. When the method calls a callback or does slow I/O, unlock before that call instead, so that other goroutines do not wait for it.

## Example

```go
s.mu.Lock()
s.mu.Unlock() // the lock is released before the update
s.count++
```

Hold the lock while you change the state:

```go
s.mu.Lock()
defer s.mu.Unlock()
s.count++
```

## Learn more

The course exercise below practises state that a mutex guards.
