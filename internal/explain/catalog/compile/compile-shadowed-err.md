---
id: compile-shadowed-err
source: compile
match: ^declared and not used: (?P<name>err)$
title: No code reads this err, so its error is lost
links:
- The Go spec: Short variable declarations | https://go.dev/ref/spec#Short_variable_declarations
- The Go spec: Declarations and scope | https://go.dev/ref/spec#Declarations_and_scope
- Effective Go: Redeclaration and reassignment | https://go.dev/doc/effective_go#redeclaration
---
## What it means

At {{file}}:{{line}} you declared a variable named `err`, and no code reads it. The error it holds is lost. Either the code never checks it, or, most often, `:=` inside an `if`, `for` or `else` block declared a second `err` while the function already has one: the new `err` hides the outer one, and the outer one keeps its old value.

## Why Go does this

`:=` declares the names on its left side in the current block. It reuses an existing variable only when that variable was declared in the same block. An `err` from the enclosing block does not count, so `x, err := f()` inside a block creates a second `err` that exists only until the closing brace. This is called shadowing. The compiler rejects the inner `err` because nothing reads it, and that saves you from an error that would vanish without a trace.

## How to find it

Go to {{file}}:{{line}} and look for an outer `err`: at the start of the function, in a named result, or before the block. Then decide what the code should do:

- Handle the error right where it happens, with `if err != nil` after the call. This is almost always right.
- When an outer `err` should get the value, assign to it: declare the other names before the block with `var`, and use `=` instead of `:=`.

## Example

```go
total, err := strconv.Atoi(a)
if err != nil {
	return 0, err
}
if b != "" {
	extra, err := strconv.Atoi(b) // declared and not used: err
	total += extra
}
return total, err // the error of the second Atoi is lost
```

Check the error where it happens:

```go
if b != "" {
	extra, err := strconv.Atoi(b)
	if err != nil {
		return 0, err
	}
	total += extra
}
```

## Learn more
