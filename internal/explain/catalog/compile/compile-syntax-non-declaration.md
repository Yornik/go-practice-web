---
id: compile-syntax-non-declaration
source: compile
match: ^syntax error: non-declaration statement outside function body$
match: ^expected declaration, found .+?(?: \(and \d+ more errors?\))?$
title: A statement is outside of any function
links:
- The Go spec: Source file organization | https://go.dev/ref/spec#Source_file_organization
- The Go spec: Short variable declarations | https://go.dev/ref/spec#Short_variable_declarations
- The Go spec: Variable declarations | https://go.dev/ref/spec#Variable_declarations
---
## What it means

At {{file}}:{{line}}, Go is at the top level of the file, outside every function, and found a statement: an assignment, a `:=`, a loop, a `return` or a stray `}`. At the top level only declarations are allowed: `import`, `const`, `var`, `type` and `func`.

## Why Go does this

A Go file is a list of declarations; statements run only inside function bodies. `:=` is a statement, so it works only inside a function. A package-level variable needs `var`, and its initial value is computed once, when the package starts.

## How to find it

Go to {{file}}:{{line}} and ask why this line is at the top level:

- A package-level variable written with `:=`: use `var count = 0`, or `var count int`.
- An extra `}` a few lines higher closed the function early, so the lines after it are outside. Count the braces from the start of the function; formatting the file after the fix shows the structure.
- The line belongs in a function, such as `init` or the function you are writing.
- This error comes right after another syntax error: fix that one first. Go may have lost track of the braces.

## Example

```go
package counter

count := 0 // syntax error: non-declaration statement outside function body

func Next() int {
	count++
	return count
}
```

Declare the package-level variable with `var`:

```go
var count = 0
```

## Learn more
