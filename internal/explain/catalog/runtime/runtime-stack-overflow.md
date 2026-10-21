---
id: runtime-stack-overflow
source: runtime
match: ^fatal error: stack overflow$
title: Endless recursion: the stack outgrew its limit in {{func}}
course: 36-tracetree
links:
- Package runtime/debug: SetMaxStack | https://pkg.go.dev/runtime/debug#SetMaxStack
- Go FAQ: Why goroutines instead of threads? | https://go.dev/doc/faq#goroutines
- The Go spec: Calls | https://go.dev/ref/spec#Calls
---
## What it means

A function called itself, directly or through other functions, and never stopped. Every call takes stack memory, and the stack grew past its limit, as the line `runtime: goroutine stack exceeds ...-byte limit` above the error says. This is a fatal error. The first frame in the exercise package is `{{func}}` at {{file}}:{{line}}.

## Why Go does this

A goroutine starts with a small stack that grows when it needs more, so deep recursion works in Go. The growth has a limit: by default 1 GB on 64-bit systems, which `runtime/debug.SetMaxStack` can change. A recursion that never ends reaches any limit, so Go stops it instead of using all memory.

## How to find it

The trace repeats the same frames and leaves out the middle. Find the cycle: 1 function, or a few that call each other. Then look at the recursive call and ask what should end the recursion:

- Is there a base case, and is it reached? An argument that does not get smaller, or a typo that passes the same value again, never reaches it.
- Does the data have a cycle that the code does not expect? A graph where `a` leads to `b` and `b` back to `a`, or a node that is its own parent, sends a plain recursive walk around forever.

Print the argument at the top of the function: the repeating values show the cycle.

## Example

```go
func depth(n *Folder) int {
	if n == nil {
		return 0
	}
	return 1 + depth(n) // n again, instead of its parent
}
```

Each call must move toward the base case:

```go
	return 1 + depth(n.Parent)
```

## Learn more

The course exercise below builds trees from flat data and walks them with recursion.
