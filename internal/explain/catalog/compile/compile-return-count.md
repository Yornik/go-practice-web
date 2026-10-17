---
id: compile-return-count
source: compile
match: ^(?P<which>too many|not enough) return values\n\s*have (?P<have>\([^\n]*\))\n\s*want (?P<want>\([^\n]*\))$
title: Wrong number of values in this return statement
links:
- The Go spec: Return statements | https://go.dev/ref/spec#Return_statements
- Effective Go: Multiple return values | https://go.dev/doc/effective_go#multiple-returns
- The Go spec: The zero value | https://go.dev/ref/spec#The_zero_value
---
## What it means

The `return` at {{file}}:{{line}} gives a different number of values than the function declares. Under the message, `have {{have}}` lists what the `return` gives and `want {{want}}` lists the declared results. `want ()` means the function declares no results at all.

## Why Go does this

In a function with results, every `return` must give exactly 1 value per result. A bare `return` without values is allowed only when the results have names. Go never fills in a missing value for you, not even `nil` for an error: you write down what the caller gets.

## How to find it

Compare `have` and `want` and look at the function's signature:

- Returning only the error from a function that returns `(T, error)`: give the zero value for the other results, such as `0`, `""`, `nil` or `T{}`.
- `want ()`: the function declares no results. Add the result types to the signature, or remove the value from the `return`.
- An extra value: count the values, and check for a stray `nil` after an edit.

## Example

```go
func ParsePort(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return err // not enough return values
	}
	return n, nil
}
```

Return a value for every result:

```go
	if err != nil {
		return 0, err
	}
```

## Learn more
