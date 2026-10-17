---
id: staticcheck-sa4004
source: staticcheck
code: SA4004
title: The loop always stops after its first iteration
links:
- staticcheck: SA4004 | https://staticcheck.dev/docs/checks/#SA4004
- The Go spec: For statements | https://go.dev/ref/spec#For_statements
---
## What it means

Every path through the body of this loop ends with a `return`, a `break` or a `panic`. So the body runs at most once, and the elements after the first are never looked at. A search loop then reports "not found" for every item except the first.

## Why Go does this

The compiler accepts a loop that cannot repeat. staticcheck follows the paths through the loop body and reports a loop whose body always leaves it. The usual cause is a statement that belongs after the loop but ended up inside it, or an `else` branch that leaves too early.

## How to find it

Go to {{file}}:{{line}}: the statement there ends the loop on every path. Ask what should happen when the current element does not match: the loop should go on to the next one. Move the statement that handles "nothing found" after the loop, so that it runs only when all elements are checked.

## Example

```go
for _, item := range items {
	if item.ID == id {
		return item, nil
	}
	return Item{}, ErrNotFound // stops after the first item
}
```

Report "not found" after the loop has seen every item:

```go
for _, item := range items {
	if item.ID == id {
		return item, nil
	}
}
return Item{}, ErrNotFound
```

## Learn more
