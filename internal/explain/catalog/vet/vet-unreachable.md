---
id: vet-unreachable
source: vet
analyzer: unreachable
match: ^unreachable code$
title: No path through the function reaches this code
links:
- The Go spec: Terminating statements | https://go.dev/ref/spec#Terminating_statements
- The Go spec: Return statements | https://go.dev/ref/spec#Return_statements
- The unreachable check of go vet | https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/unreachable
---
## What it means

The statement at {{file}}:{{line}} comes after a statement that always leaves the block: a `return`, a `panic`, a `break`, a `continue`, a `goto` or an endless `for`. Go never runs it. Often a line ended up in the wrong place, such as a log call after the `return` that it should precede, or an early `return` that you added while debugging.

## Why Go does this

The compiler accepts dead code. The Go spec defines terminating statements, after which control does not go on to the next statement. `go vet` reports code after such a statement, because it usually means the function does not do what you think.

## How to find it

Go to {{file}}:{{line}} and read the statement right above it. Ask what you meant to run:

- The code must run: move it above the statement that leaves the block.
- The code is left over: delete it.
- An early `return` from debugging: remove it, and run the tests again.

## Example

```go
if err != nil {
	return 0, err
	log.Printf("bad number %q", s) // never runs
}
```

Run the code before you leave the block:

```go
if err != nil {
	log.Printf("bad number %q", s)
	return 0, err
}
```

## Learn more
