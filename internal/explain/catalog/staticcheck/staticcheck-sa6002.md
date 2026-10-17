---
id: staticcheck-sa6002
source: staticcheck
code: SA6002
title: Putting a value that is not a pointer into a sync.Pool allocates every time
links:
- staticcheck: SA6002 | https://staticcheck.dev/docs/checks/#SA6002
- Package sync: Pool | https://pkg.go.dev/sync#Pool
---
## What it means

`sync.Pool` stores its items as `any`. A value that is not a pointer, such as a slice, must be copied to the heap to fit in an interface. So each `Put` of a slice allocates a small object, which is the kind of allocation that the pool is meant to avoid. The code works, but the pool saves less than you think.

## Why Go does this

A pool exists to reuse memory and take work away from the garbage collector. An interface value holds a pointer to its data. A pointer fits in an interface as it is, while a slice header has 3 words that need a place of their own on the heap. staticcheck reports a non-pointer argument to `Pool.Put`.

## How to find it

Go to {{file}}:{{line}}. Store a pointer to the value instead, such as a `*[]byte`, and use the same pointer type in `New` and after `Get`. First ask whether you need a pool at all: a benchmark with `-benchmem` shows whether the allocations matter for your code.

## Example

```go
var buffers = sync.Pool{New: func() any { return make([]byte, 0, 4096) }}

buffers.Put(b[:0]) // the slice header escapes to the heap
```

Pool pointers to slices:

```go
var buffers = sync.Pool{New: func() any {
	b := make([]byte, 0, 4096)
	return &b
}}

bp := buffers.Get().(*[]byte)
// use *bp, then reset and return it
*bp = (*bp)[:0]
buffers.Put(bp)
```

## Learn more
