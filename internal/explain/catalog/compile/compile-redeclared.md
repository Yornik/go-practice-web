---
id: compile-redeclared
source: compile
match: (?s)^(?P<name>\S+) redeclared in this block(?:\n.*)?$
match: ^method (?P<name>\S+) already declared(?: at .+)?$
title: {{name}} is declared twice
links:
- The Go spec: Declarations and scope | https://go.dev/ref/spec#Declarations_and_scope
- The Go spec: Method declarations | https://go.dev/ref/spec#Method_declarations
- Go FAQ: Why does Go not support overloading? | https://go.dev/doc/faq#overloading
---
## What it means

`{{name}}` is declared a second time in a scope where it already exists. The line `other declaration of` under the message, or the position after `already declared at`, points to the first declaration. For a method, the type already has a method with this name.

## Why Go does this

No name may be declared twice in the same block. The package block covers every file of the package, so 2 files of the same package share 1 set of top-level names. Go also has no overloading: 2 functions or 2 methods of 1 type cannot share a name, even with different parameters. A value receiver and a pointer receiver do not make 2 different methods either.

## How to find it

Go to {{file}}:{{line}} and to the first declaration.

- For a top-level name, search every file of the package, not only this one. An exercise can have several stub files, and a helper with a common name such as `parse` or `min` in 2 of them collides.
- Inside a function, look for a second `var` or a parameter with the same name.
- For 2 functions that do different things, give each a name that says what it does. For 2 methods that differ only in their receiver, keep 1.

## Example

```go
// parse.go
func clean(s string) string { return strings.TrimSpace(s) }

// render.go, the same package
func clean(s string) string { return strings.ToLower(s) } // clean redeclared in this block
```

Rename 1 of them after what it does:

```go
// render.go
func lowerLabel(s string) string { return strings.ToLower(s) }
```

## Learn more
