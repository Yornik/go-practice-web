---
id: runtime-out-of-memory
source: runtime
match: ^fatal error: (?:runtime: )?out of memory$
title: {{func}} asked for more memory than the system can give
course: 49-diskbuffer
links:
- The Go spec: Making slices, maps and channels | https://go.dev/ref/spec#Making_slices_maps_and_channels
- Package io: LimitReader | https://pkg.go.dev/io#LimitReader
- A Guide to the Go Garbage Collector | https://go.dev/doc/gc-guide
---
## What it means

An allocation failed: Go asked the operating system for memory and did not get it. If the line above the error reads `runtime: out of memory: cannot allocate N-byte block`, N is the size of the single allocation that failed. This is a fatal error: the program stopped, and no deferred function ran. The first frame in the exercise package is `{{func}}` at {{file}}:{{line}}.

## Why Go does this

`make([]byte, n)` asks for all n bytes at once. Go accepts any size that fits the address space, so a wrong number leads straight to a huge request. Sizes that come from input, such as a length field in a file or a header, are the usual cause: a corrupt or hostile input can say "4 GB". A slow leak, such as a map that only grows, usually ends differently: the operating system or the container kills the process, and Go prints nothing.

## How to find it

Look at the allocation in `{{func}}`: a `make`, an `append` in a loop, `io.ReadAll`, a growing buffer. Print the size right before it. Then ask where that number comes from:

1. Is it read from input? Check it against a maximum and against the bytes that are really there, before you allocate.
2. Is it computed? Look for a subtraction that wrapped around in an unsigned type, or a multiplication with the wrong unit.
3. Does input of unknown size get read all at once? Read it in parts, or wrap the reader in `io.LimitReader`.

## Example

```go
n := binary.BigEndian.Uint32(header) // from the file, may be garbage
payload := make([]byte, n)
```

Check the length before you trust it:

```go
if n > maxPayload || int(n) > len(rest) {
	return nil, errBadLength
}
payload := make([]byte, n)
```

## Learn more
