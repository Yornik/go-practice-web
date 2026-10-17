---
id: compile-undefined
source: compile
match: ^undefined: (?P<name>[^\s(]+)(?: \(but have (?P<have>[^)]+)\))?$
title: {{name}} is not declared where you use it
links:
- The Go spec: Declarations and scope | https://go.dev/ref/spec#Declarations_and_scope
- The Go spec: Exported identifiers | https://go.dev/ref/spec#Exported_identifiers
- The Go spec: Blocks | https://go.dev/ref/spec#Blocks
---
## What it means

The compiler reached the name `{{name}}` and found no declaration of it that is visible at this line. Go resolves every name when it compiles, so the build stops here. With a package prefix, such as `strings.contains`, the package is there but it does not export that name. When the message ends with `(but have X)`, Go found a name that differs only in upper or lower case.

## Why Go does this

Every name is visible only inside the block that declares it: the package, the file for imports, a function, or a block such as the body of an `if` or `for`. A variable that you declare with `:=` in an `if` or `for` statement does not exist after its closing brace. A name from another package is visible only when it starts with an upper-case letter: it is exported.

## How to find it

Go to {{file}}:{{line}} and check, in this order:

1. Spelling and case. `Count` and `count` are 2 different names.
2. Scope. Find the declaration. Does its block end before this line?
3. Imports. For a package name such as `strings`, is the import there?
4. Existence. Search the stub and the test files: maybe you still have to write `{{name}}`.

## Example

```go
if n, err := strconv.Atoi(s); err == nil {
	total += n
}
return n // undefined: n, because n exists only inside the if
```

Declare the variable before the block when you need it after the block:

```go
n, err := strconv.Atoi(s)
if err != nil {
	return 0
}
return n
```

## Learn more
