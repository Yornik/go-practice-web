---
id: compile-assignment-mismatch
source: compile
match: ^assignment mismatch: (?P<vars>\d+) variables? but (?:(?P<call>.+) returns )?(?P<values>\d+) values?$
title: The number of variables does not match the number of values
links:
- The Go spec: Assignment statements | https://go.dev/ref/spec#Assignment_statements
- Effective Go: Multiple return values | https://go.dev/doc/effective_go#multiple-returns
- Effective Go: The blank identifier in multiple assignment | https://go.dev/doc/effective_go#blank_assign
---
## What it means

At {{file}}:{{line}}, the number of variables on the left side, {{vars}}, differs from the number of values on the right side, {{values}}. When the message says `returns`, the right side is 1 call that returns several values, and you must take all of them.

## Why Go does this

Every value needs a place to go. A function that returns `(int, error)` gives 2 values, so an assignment from it needs 2 names. Go never drops a result silently, because the dropped result is often the error. Write `_`, the blank identifier, for a value you really do not need.

## How to find it

- Look up the results of the function, for example with `go doc strconv.Atoi`, and write 1 name per result.
- Before you put `_` in place of an error, decide what your code should do when the call fails. Usually it should return the error.
- With more variables than values, such as `a, b := f()` where `f` returns 1 value: only a map index, a type assertion and a channel receive give a second `ok` value, in the form `v, ok := m[k]`. A normal function does not.

## Example

```go
n := strconv.Atoi(s) // assignment mismatch: 1 variable but strconv.Atoi returns 2 values
```

Take both results and handle the error:

```go
n, err := strconv.Atoi(s)
if err != nil {
	return 0, fmt.Errorf("port %q: %w", s, err)
}
```

## Learn more
