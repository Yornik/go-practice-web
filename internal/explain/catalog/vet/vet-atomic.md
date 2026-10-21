---
id: vet-atomic
source: vet
analyzer: atomic
match: ^direct assignment to atomic value$
title: The result of an atomic operation is written back with a plain assignment
course: 46-broadcaster
links:
- Package sync/atomic | https://pkg.go.dev/sync/atomic
- The Go Memory Model | https://go.dev/ref/mem
- The atomic check of go vet | https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/atomic
---
## What it means

You call a function such as `atomic.AddInt64(&x, 1)` and assign its result to `x`. The Add already stored the new value, atomically. The assignment then writes `x` a second time, without atomic access. When another goroutine adds between the 2 writes, your plain write overwrites its update with an older value. Counts get lost, and `-race` reports a data race.

## Why Go does this

Atomic operations protect a variable only when every access to it is atomic. The Go memory model calls a write that runs at the same time as another access to the same variable a data race, unless all of the accesses are atomic. The compiler cannot know that a variable is meant to be atomic, so `go vet` looks for this pattern. `go test` runs this check before the tests.

## How to find it

Go to {{file}}:{{line}}. Drop the assignment and call the atomic function as a statement. If you need the new value, store it in a local variable. Better still, use the atomic types such as `atomic.Int64`. Their methods make a plain read or write of the value impossible, so this mistake cannot happen.

## Example

```go
var requests int64

func Hit() {
	requests = atomic.AddInt64(&requests, 1) // a second, plain write
}
```

Use an atomic type, so every access goes through its methods:

```go
var requests atomic.Int64

func Hit() {
	requests.Add(1)
}
```

## Learn more

The course exercise below practises sync/atomic next to a mutex.
