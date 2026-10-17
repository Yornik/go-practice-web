---
id: compile-multiple-value
source: compile
match: ^multiple-value (?P<call>.+) \(value of type (?P<type>\(.+\))\) in single-value context$
title: {{call}} returns several values, but this place takes only 1
links:
- The Go spec: Calls | https://go.dev/ref/spec#Calls
- Effective Go: Multiple return values | https://go.dev/doc/effective_go#multiple-returns
- The Go blog: Error handling and Go | https://go.dev/blog/error-handling-and-go
---
## What it means

`{{call}}` returns more than 1 value, of the types `{{type}}`. At {{file}}:{{line}} you used the call where Go needs exactly 1 value: as an operand of an operator, as 1 of several arguments, inside a composite literal or in a `return` with other values. Go does not pick the first result for you.

## Why Go does this

A call with several results can appear in only 3 places: on the right side of an assignment with 1 variable per result, as the only argument of another call whose parameters match the results, as in `f(g())`, or as the only value of a `return` in a function with the same results. Most such functions return an error as their last result. Using the call as a single value would throw that error away, so Go makes you take the results apart and look at the error.

## How to find it

Split the expression into steps: assign the results of `{{call}}` to variables, check the error, then use the value. If you are not sure what the call returns, read its signature with `go doc`.

## Example

```go
func Double(s string) (int, error) {
	return 2 * strconv.Atoi(s), nil // multiple-value strconv.Atoi(s) (value of type (int, error)) in single-value context
}
```

Take the results apart first:

```go
func Double(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, err
	}
	return 2 * n, nil
}
```

## Learn more
