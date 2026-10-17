---
id: compile-declared-and-not-used
source: compile
match: ^declared and not used: (?P<name>[^e\s]\S*|e[^r\s]\S*|er[^r\s]\S*|err\S+|er?)$
match: ^(?P<name>\S+) declared and not used$
title: The variable {{name}} is declared but its value is never read
links:
- The Go spec: Variable declarations | https://go.dev/ref/spec#Variable_declarations
- The Go spec: Short variable declarations | https://go.dev/ref/spec#Short_variable_declarations
- Effective Go: Unused imports and variables | https://go.dev/doc/effective_go#blank_unused
---
## What it means

You declared the local variable `{{name}}`, but no code reads its value. Assigning a new value to it does not count as a use; only reading it does. Go does not compile a function that has an unused local variable.

## Why Go does this

The Go spec allows a compiler to reject a variable that is declared inside a function body and never used, and the Go compiler does. An unused variable is often a bug: a result you computed and then forgot, or a second variable that `:=` created where you meant to assign to the first one. Package-level variables and function parameters are exempt.

## How to find it

Go to {{file}}:{{line}}, where `{{name}}` is declared, and ask:

- Did you mean to return it or pass it on?
- Is this a `:=` inside an `if`, `for` or `switch` block, while a variable with the same name exists outside it? Then `:=` declared a new variable that hides the outer one. Use `=` to assign to the outer variable.
- Is it the variable of a type switch, `switch {{name}} := x.(type)`, that no case uses? Then write `switch x.(type)`.
- Is it left over from debugging? Delete it. While you work, `_ = {{name}}` silences the error, but remove that line before you finish.

## Example

```go
count := 0
for _, line := range lines {
	if line != "" {
		count := count + 1 // declared and not used: a new count
	}
}
return count // always 0
```

Assign with `=`, so the loop updates the variable outside it:

```go
	if line != "" {
		count = count + 1
	}
```

## Learn more
