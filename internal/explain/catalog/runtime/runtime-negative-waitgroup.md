---
id: runtime-negative-waitgroup
source: runtime
match: ^panic: sync: negative WaitGroup counter$
title: WaitGroup.Done ran more often than Add in {{func}}
course: 12-workerpool
links:
- Package sync: WaitGroup | https://pkg.go.dev/sync#WaitGroup
- Package sync: WaitGroup.Add | https://pkg.go.dev/sync#WaitGroup.Add
- The Go blog: Pipelines and cancellation | https://go.dev/blog/pipelines#fan-out-fan-in
---
## What it means

The counter of a `sync.WaitGroup` went below 0: `Done` was called more times than `Add` added. It happened in `{{func}}` at {{file}}:{{line}}. If that line is a closing brace `}`, the extra `Done` was a deferred call that ran when the function returned.

## Why Go does this

A WaitGroup counts work that is still running: `Add(n)` adds n, `Done` subtracts 1, and `Wait` blocks until the counter is 0. The docs say that `Add` panics when the counter goes negative. A negative counter means the counting is wrong, and `Wait` may already have returned while work still runs. Go stops here instead of hiding that.

## How to find it

Find every `Done` for this WaitGroup and pair each one with an `Add`. Look for:

- `defer wg.Done()` and an explicit `wg.Done()` in the same goroutine.
- `Done` inside a loop, or on a path that runs twice, such as a retry.
- 1 `Add(1)` for several goroutines that each call `Done`.
- A WaitGroup passed or copied by value: the copy has its own counter, so its `Done` can go below 0 while the original never reaches 0. `go vet` reports that copy.

The safe shape: `wg.Add(1)` right before the `go` statement, and `defer wg.Done()` as the first line of the goroutine, with no other `Done`.

## Example

```go
wg.Add(1)
go func() {
	defer wg.Done()
	process(item)
	wg.Done() // a second Done: the deferred one makes the counter -1
}()
```

Keep only the deferred `Done`:

```go
wg.Add(1)
go func() {
	defer wg.Done()
	process(item)
}()
```

## Learn more
