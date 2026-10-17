---
id: vet-waitgroup
source: vet
analyzer: waitgroup
match: ^WaitGroup\.Add called from inside new goroutine$
title: WaitGroup.Add runs inside the new goroutine, so Wait may return too early
course: 12-workerpool
links:
- Package sync: WaitGroup.Add | https://pkg.go.dev/sync#WaitGroup.Add
- The Go Memory Model | https://go.dev/ref/mem
- The waitgroup check of go vet | https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/waitgroup
---
## What it means

The goroutine calls `Add` on the WaitGroup that counts it. A `go` statement only starts a goroutine; the goroutine may not have run its first line when the code after the loop reaches `Wait`. `Wait` then sees a counter of 0 and returns while work is still running. You get missing results, or a data race on them, and only in some runs.

## Why Go does this

The docs of `WaitGroup.Add` say that an `Add` with a positive delta while the counter is 0 must happen before `Wait`, and that this typically means calling `Add` before the statement that creates the goroutine. The scheduler decides when a new goroutine runs. The compiler cannot see the order, so `go vet` reports an `Add` inside the function literal of a `go` statement.

## How to find it

Go to {{file}}:{{line}}. Move the `wg.Add(1)` out of the function literal, to the line right before the `go` statement. Keep `defer wg.Done()` as the first line inside the goroutine. The current docs also show `WaitGroup.Go`, which does both. It needs Go 1.25, and the course module says go 1.24, so `go vet` would report it.

## Example

```go
for _, item := range items {
	go func() {
		wg.Add(1) // may run after wg.Wait
		defer wg.Done()
		handle(item)
	}()
}
wg.Wait()
```

Count the goroutine before you start it:

```go
for _, item := range items {
	wg.Add(1)
	go func() {
		defer wg.Done()
		handle(item)
	}()
}
```

## Learn more

The course exercise below practises starting goroutines and waiting for all of them.
