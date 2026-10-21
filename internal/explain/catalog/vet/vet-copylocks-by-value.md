---
id: vet-copylocks-by-value
source: vet
analyzer: copylocks
match: ^(?P<func>\S+) passes lock by value: (?P<what>.+)$
title: {{func}} gets a copy of a lock, so the lock protects nothing
course: 33-ratelimit
links:
- Package sync: Mutex | https://pkg.go.dev/sync#Mutex
- Effective Go: Pointers vs. Values | https://go.dev/doc/effective_go#pointers_vs_values
- The copylocks check of go vet | https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/copylock
---
## What it means

`{{func}}` takes a value that is a lock or contains one: {{what}}. Go passes arguments and value receivers by copying them, so every call gets its own copy of the lock. 2 calls that run at the same time each lock their own copy, and nothing stops them from changing the shared data together. With a value receiver, the changes that the method makes to plain fields are also lost when it returns.

## Why Go does this

The docs of package sync say that a `Mutex` must not be copied after first use. A lock works only when every goroutine uses the same lock value. The compiler copies structs without asking, so `go vet` checks every place that copies a value of a type that holds a `sync.Mutex`, `sync.RWMutex`, `sync.WaitGroup` or another lock.

## How to find it

Go to {{file}}:{{line}}. Look at the receiver or the parameter of `{{func}}`: its type has no `*`. Ask who owns the value. When the type holds a lock, there is 1 value that every caller shares, so pass a pointer to it. Use pointer receivers for every method of that type, not only for this one, so the method set stays consistent.

## Example

```go
type Counter struct {
	mu   sync.Mutex
	hits map[string]int
}

func (c Counter) Inc(path string) { // c is a copy, and so is c.mu
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hits[path]++
}
```

Use a pointer receiver, so every call locks the same mutex:

```go
func (c *Counter) Inc(path string) {
```

## Learn more

The course exercise below practises a type that guards its state with a mutex.
