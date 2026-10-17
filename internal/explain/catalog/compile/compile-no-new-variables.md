---
id: compile-no-new-variables
source: compile
match: ^no new variables on left side of :=$
title: Every name left of := already exists, so := has nothing to declare
links:
- The Go spec: Short variable declarations | https://go.dev/ref/spec#Short_variable_declarations
- The Go spec: Assignment statements | https://go.dev/ref/spec#Assignment_statements
- Effective Go: Redeclaration and reassignment | https://go.dev/doc/effective_go#redeclaration
---
## What it means

`:=` declares new variables. At {{file}}:{{line}}, every name on its left side was already declared earlier in the same block, so there is nothing new to declare. You almost certainly meant to give an existing variable a new value, and that is `=`.

## Why Go does this

`:=` may reuse an existing variable only when at least 1 other name on its left side is new, and only when the existing variable was declared in the same block. Then it assigns to the existing variable. That is why `n, err := f()` followed by `m, err := g()` compiles: `m` is new, and `err` is assigned. With no new name, Go reports the statement, because a declaration that declares nothing is a mistake.

## How to find it

Go to {{file}}:{{line}} and find the first declaration of each name on the left side, higher up in the same block.

- To update the variable, replace `:=` with `=`.
- To keep the old value and have a second one, give the new variable its own name.

The opposite mistake does not give this error: inside an inner block, such as the body of an `if` or `for`, `:=` always creates new variables that hide the outer ones.

## Example

```go
count := 0
for _, line := range lines {
	if line != "" {
		count++
	}
}
count := count * 2 // no new variables on left side of :=
```

Assign to the variable that exists:

```go
count = count * 2
```

## Learn more
