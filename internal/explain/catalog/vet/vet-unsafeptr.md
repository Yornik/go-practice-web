---
id: vet-unsafeptr
source: vet
analyzer: unsafeptr
match: ^possible misuse of (?P<what>\S+)$
title: This use of {{what}} breaks the rules of package unsafe
links:
- Package unsafe: Pointer | https://pkg.go.dev/unsafe#Pointer
- The unsafeptr check of go vet | https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/unsafeptr
---
## What it means

The code at {{file}}:{{line}} uses `{{what}}` in a way that the rules of package unsafe do not allow. Most often it turns an integer address, a `uintptr`, back into a pointer. A `uintptr` is a plain number. The garbage collector does not know that it points to memory, so that memory may be moved or freed before you use the pointer. The program can then crash or corrupt data, rarely, and far away from this line.

## Why Go does this

Package unsafe steps outside the type system, and its docs list the only valid patterns for conversions between `unsafe.Pointer` and `uintptr`. For example, the conversion to `uintptr`, the arithmetic and the conversion back must happen in 1 expression. The compiler cannot check those rules, so `go vet` reports conversions that fit none of the patterns.

## How to find it

Go to {{file}}:{{line}} and ask whether you need package unsafe at all. The course needs none of it: slices, indexes and package encoding/binary cover byte layouts. Keep a pointer or a slice where you now keep an address, and index into it. When you do need pointer arithmetic, use `unsafe.Add`, which keeps the value a pointer.

## Example

```go
addr := uintptr(unsafe.Pointer(&buf[0]))
next := (*byte)(unsafe.Pointer(addr + 1)) // addr is only a number by now
```

Index the slice instead:

```go
next := &buf[1]
```

## Learn more
