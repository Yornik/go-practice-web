---
id: test-did-not-return
source: test
match: ^(?P<call>[^\n]+?) did not (?P<action>[^\n]+?) within (?P<limit>(?:\d+(?:\.\d+)?(?:ns|µs|us|ms|s|m|h))+)(?:\W[^\n]*)?(?:\n(?s:.*))?$
match: ^(?P<call>[^\n]+?): no (?P<action>answer) within (?P<limit>(?:\d+(?:\.\d+)?(?:ns|µs|us|ms|s|m|h))+)(?:\W[^\n]*)?(?:\n(?s:.*))?$
title: {{call}} did not {{action}} in time: something blocks
course: 34-cache
links:
- Package sync: Mutex.Lock | https://pkg.go.dev/sync#Mutex.Lock
- Effective Go: Channels | https://go.dev/doc/effective_go#channels
- The Go spec: Select statements | https://go.dev/ref/spec#Select_statements
---
## What it means

The test started `{{call}}` and waited {{limit}} for it to {{action}}. That did not happen, so the test gave up at {{file}}:{{line}}. The course tests do this on purpose: a blocked call fails 1 test with a clear message instead of hanging the whole run. Your code is not slow: it waits for something that never comes.

## Why Go does this

A goroutine that waits on a lock, a channel or a `WaitGroup` sleeps until another goroutine wakes it. If none ever will, it sleeps forever, without an error. The test only sees the time pass.

## How to find it

Follow `{{call}}` into your code and look at every operation that can wait:

- `mu.Lock()`: does this goroutine already hold the lock? `Lock` blocks even for the goroutine that holds it, so a method that locks and then calls another method that locks waits forever. Is there a path, such as an early `return`, that leaves the lock held?
- A send on a channel: who receives it? A receive: who sends or closes?
- `wg.Wait()`: does every `Add` get its `Done`, also on error paths?
- A `select` without a ready case and without `default`.

Print a line before and after each wait and run only this test: the last line printed is the wait that never ends.

## Example

```go
func (c *Counter) Inc() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.n = c.Get() + 1 // Get locks c.mu again: blocks forever
}
```

Read the field directly while you hold the lock, or call a helper that expects the lock to be held:

```go
	c.n++
```

## Learn more

The course exercise below practises methods that share 1 lock.
