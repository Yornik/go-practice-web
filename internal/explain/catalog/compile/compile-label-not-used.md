---
id: compile-label-not-used
source: compile
match: ^label (?P<label>\S+) (?:defined|declared) and not used$
title: The label {{label}} is never used by a break, continue or goto
links:
- The Go spec: Label scopes | https://go.dev/ref/spec#Label_scopes
- The Go spec: Break statements | https://go.dev/ref/spec#Break_statements
- The Go spec: Labeled statements | https://go.dev/ref/spec#Labeled_statements
---
## What it means

You put the label `{{label}}` on a statement at {{file}}:{{line}}, but no `break {{label}}`, `continue {{label}}` or `goto {{label}}` refers to it. Go does not allow an unused label.

## Why Go does this

The spec makes an unused label illegal, like an unused variable or import. A label is almost always written for a reason: to leave or continue an outer loop from an inner one. When nothing uses it, the `break` or `continue` that should use it is often missing its label. Then it acts on the innermost loop, which is a real bug.

## How to find it

Go to {{file}}:{{line}} and look at the `break` and `continue` statements inside the labeled loop:

- A plain `break` inside an inner loop, a `switch` or a `select` leaves only that inner statement. To leave the labeled loop, write `break {{label}}`.
- The same holds for `continue {{label}}`, which starts the next iteration of the outer loop.
- If no statement needs to reach the outer loop, delete the label.

## Example

```go
search:
	for i, line := range grid { // label search defined and not used
		for j, v := range line {
			if v == want {
				row, col = i, j
				break // leaves only the inner loop
			}
		}
	}
```

Name the loop you want to leave:

```go
			if v == want {
				row, col = i, j
				break search
			}
```

## Learn more
