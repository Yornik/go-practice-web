---
id: vet-copylocks
source: vet
analyzer: copylocks
title: This line copies a value that holds a lock
course: 33-ratelimit
links:
- Package sync: Mutex | https://pkg.go.dev/sync#Mutex
- The Go spec: For statements with range clause | https://go.dev/ref/spec#For_range
- The copylocks check of go vet | https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/copylock
---
## What it means

The code at {{file}}:{{line}} copies a value whose type is a lock, such as `sync.Mutex`, or contains one. The message says how it copies: an assignment, a `range` variable, a return value, a call argument or a composite literal. The copy has its own lock. Code that locks the copy does not block code that locks the original, so the lock no longer protects the shared data.

## Why Go does this

The docs of package sync say that a `Mutex` must not be copied after first use, and the same holds for `RWMutex`, `WaitGroup`, `Once` and `Cond`. Go copies structs in every assignment without asking. `go vet` finds the copies that the compiler accepts.

## How to find it

Go to {{file}}:{{line}} and find the value that the message names. Then ask where the original lives and who shares it:

- A `range` loop over a slice of structs copies each element into the loop variable. Range over the index and use `&items[i]`, or keep pointers in the slice.
- An assignment `x := *p` or a return of the struct copies it. Pass and return the pointer instead.
- A parameter or receiver without `*` copies it at every call.

## Example

```go
for _, c := range conns { // c is a copy of each Conn, lock included
	if c.open {
		n++
	}
}
```

Use the index, so you read the element where it lives:

```go
for i := range conns {
	if conns[i].open {
		n++
	}
}
```

## Learn more
