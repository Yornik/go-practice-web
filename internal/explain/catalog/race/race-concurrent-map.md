---
id: race-concurrent-map
source: race
match: (?s)^WARNING: DATA RACE\n.*?\n  runtime\.map\w*\(\)\n
title: Data race on a map in {{test}}
course: 20-counters
links:
- Go FAQ: Why are map operations not defined to be atomic? | https://go.dev/doc/faq#atomic_maps
- The Go blog: Go maps in action | https://go.dev/blog/maps#concurrency
- Data Race Detector: report format | https://go.dev/doc/articles/race_detector#Report_Format
- Package sync: Map | https://pkg.go.dev/sync#Map
---
## What it means

2 goroutines used the same map without anything that orders their accesses, and at least 1 of them wrote. The race detector caught a {{access}} of the map that conflicts with an earlier {{prev_access}}. The frames named `runtime.mapassign...` (a write), `runtime.mapaccess...` (a read) or `runtime.mapdelete...` are the map operations themselves; your code is the frame below them.

## Why Go does this

Go maps are not safe for concurrent use. A write can rearrange the whole table, for example when the map grows, so even goroutines that use different keys conflict. Without `-race` the same bug sometimes stops the program with `fatal error: concurrent map writes`, and sometimes it silently loses entries. The race detector reports 2 accesses that nothing orders, even when they did not happen at the same moment.

## How to find it

In each of the first 2 blocks, skip the `runtime.map...` frames: the next frame in the exercise package is the line that reads or writes the map. If a block shows only runtime frames, use the other block and the "created at" blocks. Then list every place that touches this map, and give all of them the same protection, reads and `len` included:

- 1 `sync.Mutex` or `sync.RWMutex` that every access holds.
- Or 1 goroutine that owns the map and gets requests through a channel.

`sync.Map` fits only the special cases its docs describe.

## Example

```go
status := make(map[string]int)
for _, u := range urls {
	go func() {
		status[u] = check(u) // every goroutine writes the map
	}()
}
```

Hold 1 lock for every access:

```go
var mu sync.Mutex
// in each goroutine:
code := check(u)
mu.Lock()
status[u] = code
mu.Unlock()
```

## Learn more

The course exercise below practises a map that many goroutines share behind 1 lock.
