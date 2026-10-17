---
id: compile-missing-return
source: compile
match: ^missing return$
title: The function can reach its end without a return statement
links:
- The Go spec: Terminating statements | https://go.dev/ref/spec#Terminating_statements
- The Go spec: Return statements | https://go.dev/ref/spec#Return_statements
- The Go spec: Function declarations | https://go.dev/ref/spec#Function_declarations
---
## What it means

The function declares results, but Go found a path through its body that reaches the closing brace without a `return`. The position {{file}}:{{line}} points at that closing brace. Go does not invent a value for this path, so the function does not compile.

## Why Go does this

The body of a function with results must end in a terminating statement: a `return`, a `panic` call, an `if` whose `if` and `else` branches both end in one, a `for` without a condition and without a `break`, or a `switch` with a `default` where every case ends in one. Go checks this from the shape of the code alone. It does not reason about values: a loop that you know always returns from inside still counts as a loop that can end.

## How to find it

Read the function from its last statement backwards and follow each path to the closing brace:

- A loop that returns when it finds something: what should the function return when it finds nothing? Add that `return` after the loop.
- An `if` or `else if` chain without a final `else`.
- A `switch` without a `default`.

Decide on the honest answer for that path: a zero value with an error, `-1`, `false`. Use `panic` only for a path that truly cannot happen.

## Example

```go
func Index(xs []string, want string) int {
	for i, x := range xs {
		if x == want {
			return i
		}
	}
} // missing return
```

Return the answer for "not found" after the loop:

```go
	}
	return -1
}
```

## Learn more
