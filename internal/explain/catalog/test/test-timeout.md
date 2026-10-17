---
id: test-timeout
source: test
match: ^panic: test timed out after (?P<after>\S+)$
title: {{test}} was still running after {{after}}
course: 12-workerpool
links:
- go test flags: -timeout | https://pkg.go.dev/cmd/go#hdr-Testing_flags
- The Go blog: Pipelines and cancellation | https://go.dev/blog/pipelines#stopping-short
- Effective Go: Channels | https://go.dev/doc/effective_go#channels
---
## What it means

`go test` stopped the whole test binary, because {{test}} had not finished after {{after}}. The lines under `running tests:` name the tests that were still running. A test that needs this long is rarely slow: it waits for something that never happens.

## Why Go does this

A goroutine that waits on a channel, a lock or a `WaitGroup` sleeps without an error until another goroutine wakes it. If nothing ever will, it sleeps forever. The runtime reports a deadlock only when every goroutine sleeps, and in a test binary the timer of `-timeout` still counts as something that can happen. So the test hangs until the timeout fires. Then the testing package panics and prints every goroutine, so you can see where each one waits.

## How to find it

Skip the goroutine of the alarm (`testing.(*M).startAlarm`), goroutine 1, the test runner, and a parent test that waits in `testing.(*T).Run` for its subtest. Look for the goroutines in the exercise package; the test that hangs waits at {{file}}:{{line}}. The word in brackets says what each one waits for: `[chan receive]`, `[chan send]`, `[select]`, `[sync.Mutex.Lock]`, `[sync.WaitGroup.Wait]`. For each, ask which goroutine should wake it, and why that never happens:

- A receive that expects more values than the senders send.
- A send whose receiver already returned.
- A goroutine that returned early, on an error path, without its send or its `Done`.
- A lock that is never released.

## Example

```go
ch := make(chan int)
go func() { ch <- add(xs[:half]) }()
go func() { ch <- add(xs[half:]) }()
return <-ch + <-ch + <-ch // 2 senders, 3 receives: the third waits forever
```

Receive exactly once per send:

```go
return <-ch + <-ch
```

## Learn more
