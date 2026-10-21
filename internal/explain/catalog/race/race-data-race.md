---
id: race-data-race
source: race
match: ^WARNING: DATA RACE
fallback: true
title: Data race in {{func}}
course: 20-counters
links:
- Data Race Detector | https://go.dev/doc/articles/race_detector
- The Go Memory Model | https://go.dev/ref/mem
- Package sync | https://pkg.go.dev/sync
---
## What it means

2 goroutines used the same memory at the same time, and at least 1 of them wrote to it. The race detector caught a {{access}} in `{{func}}` ({{file}}:{{line}}) that conflicts with an earlier {{prev_access}} in `{{prev_func}}` ({{prev_file}}:{{prev_line}}). Nothing put the 2 accesses in order. The result can be a lost update or a corrupted value, and it changes from run to run.

## Why Go does this

The Go memory model says what one goroutine sees of another goroutine's writes only when something synchronizes them: a mutex, a channel operation, `sync.WaitGroup`, or `sync/atomic`. Without that, the compiler and the CPU may reorder or cache memory accesses. `-race` watches every memory access while the tests run and reports the conflicts it sees. It cannot prove that code has no races, so a clean run is not proof.

## How to find it

Read the report from the top. The first block is the access that was caught; the block that starts with "Previous" is the access it conflicts with. In each block, the first frame in the exercise package is the line to look at. The "created at" blocks show where each goroutine was started. Name the shared variable, then choose 1 fix:

- Protect it with a `sync.Mutex` that every access takes.
- Hand the value over through a channel, so only 1 goroutine owns it at a time.
- Give each goroutine its own variable, and combine the results after `wg.Wait()`.

## Example

```go
hits := 0
var wg sync.WaitGroup
for range 10 {
	wg.Add(1)
	go func() {
		defer wg.Done()
		hits++ // 10 goroutines read and write hits
	}()
}
wg.Wait()
```

Let a mutex order the accesses:

```go
var mu sync.Mutex
// in each goroutine:
mu.Lock()
hits++
mu.Unlock()
```

## Learn more

The course exercise below practises counters that goroutines share, checked with `-race`.
