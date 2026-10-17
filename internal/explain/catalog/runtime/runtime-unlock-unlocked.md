---
id: runtime-unlock-unlocked
source: runtime
match: ^fatal error: sync: (?P<call>unlock|RUnlock|Unlock) of unlocked (?P<lock>mutex|RWMutex)$
title: {{call}} of a {{lock}} that is not locked, in {{func}}
course: 09-cache
links:
- Package sync: Mutex.Unlock | https://pkg.go.dev/sync#Mutex.Unlock
- Package sync: RWMutex.RUnlock | https://pkg.go.dev/sync#RWMutex.RUnlock
- Effective Go: Defer | https://go.dev/doc/effective_go#defer
---
## What it means

`{{func}}` released a lock at {{file}}:{{line}} that was not held at that moment. If that line is a closing brace `}`, the unlock was a deferred call. This is a fatal error: `recover` cannot catch it, and the program stops.

## Why Go does this

The docs of package sync say that unlocking a mutex that is not locked is a run-time error. An extra unlock would later let 2 goroutines into code that only 1 may run at a time, so Go stops at once. A `sync.RWMutex` has 2 pairs: `RLock` goes with `RUnlock`, and `Lock` goes with `Unlock`. Mixing them is the same mistake, and the message names the call that failed.

## How to find it

Read `{{func}}` and count the lock and unlock calls on every path through it, returns inside `if` blocks and deferred calls included. Look for:

- `defer mu.Unlock()` and also an explicit `mu.Unlock()`, for example before a call that must run without the lock.
- `RLock` paired with `defer mu.Unlock()`, or `Lock` with `RUnlock`.
- An unlock in a helper function and again in its caller.

The safest shape is `mu.Lock()` followed by `defer mu.Unlock()` on the next line, and no other unlock in that function. When you must release the lock early, do not also defer it.

## Example

```go
func (r *Registry) Lookup(name string) string {
	r.mu.RLock()
	defer r.mu.Unlock() // RLock needs RUnlock
	return r.addrs[name]
}
```

Pair each lock with its own unlock:

```go
	r.mu.RLock()
	defer r.mu.RUnlock()
```

## Learn more
