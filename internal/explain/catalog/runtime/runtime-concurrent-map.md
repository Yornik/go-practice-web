---
id: runtime-concurrent-map
source: runtime
match: ^fatal error: concurrent map (?P<op>writes|read and map write|iteration and map write|clone and map write)$
title: Concurrent map {{op}} in {{func}}
course: 09-cache
links:
- Go FAQ: Why are map operations not defined to be atomic? | https://go.dev/doc/faq#atomic_maps
- The Go blog: Go maps in action | https://go.dev/blog/maps#concurrency
- Package sync: Mutex | https://pkg.go.dev/sync#Mutex
- Data Race Detector | https://go.dev/doc/articles/race_detector
---
## What it means

2 goroutines used the same map at the same moment, and at least 1 of them wrote to it. The runtime caught it in `{{func}}` at {{file}}:{{line}} and stopped the program. This is a fatal error, not a panic: `recover` cannot catch it, and deferred functions do not run.

## Why Go does this

Go maps are not safe for concurrent use. A write can rearrange the whole table, for example when the map grows, so even writes to different keys conflict, and a read during a write can see the table in the middle of a change. Locking every map access would slow down the common case, so Go leaves the locking to you. The runtime has a cheap check that catches some overlapping accesses and stops the program before it uses a corrupt map. It does not catch all of them; `go test -race` finds them reliably.

## How to find it

The trace shows only the goroutine that noticed the conflict. The other one is missing. List every goroutine that can reach this map: each `go` statement, and each method that callers use from several goroutines at once. Every access needs the same protection, reads and `len` included:

- 1 `sync.Mutex` (or `sync.RWMutex`) that every method holds while it touches the map.
- Or 1 goroutine that owns the map, and gets requests through a channel.

Run the test with `-race` to see both accesses.

## Example

```go
counts := make(map[string]int)
for _, w := range words {
	wg.Add(1)
	go func() {
		defer wg.Done()
		counts[w]++ // many goroutines write the same map
	}()
}
```

Guard every access with 1 mutex:

```go
var mu sync.Mutex
// in each goroutine:
mu.Lock()
counts[w]++
mu.Unlock()
```

## Learn more

The course exercise below practises a map that many goroutines share behind 1 lock.
