---
id: guide-race-report
source: guide
title: How to read a race report
course: 37-workerpool
links:
- Data Race Detector | https://go.dev/doc/articles/race_detector
- The Go Memory Model | https://go.dev/ref/mem
- The Go blog: Introducing the Go Race Detector | https://go.dev/blog/race-detector
---
## What a report says

With `-race`, Go watches every memory access while the tests run. When 2 goroutines touch the same memory, at least 1 of them writes, and nothing orders the 2 accesses, it prints a report between 2 lines of `=` signs. The test that was running fails with `race detected during execution of test`.

## The 4 blocks

```text
WARNING: DATA RACE
Write at 0x00c000012345 by goroutine 8:
  ...frames...
Previous read at 0x00c000012345 by goroutine 7:
  ...frames...
Goroutine 8 (running) created at:
  ...frames...
Goroutine 7 (finished) created at:
  ...frames...
```

1. The access that was caught: a read or a write, the address, and the goroutine.
2. The earlier access to the same address by another goroutine.
3. and 4. Where each of the 2 goroutines was started.

In every block, skip the frames of `runtime`, `sync` and `testing`. The first frame in the exercise package is the line that matters. The same address in both blocks means the same variable, often a field of a struct, a map or a slice element.

## What to fix

Name the shared variable, then decide who owns it. Protect it with a `sync.Mutex` that every access takes, pass it through a channel so only 1 goroutine owns it at a time, or give each goroutine its own copy and combine the results after `wg.Wait()`. `sync/atomic` fits a single counter or flag. A `time.Sleep` does not fix a race: it only makes it rarer.

The detector finds only the races that happen during this run. Run the tests with `-race` often, and more than once.

## Learn more
