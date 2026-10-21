---
id: staticcheck-sa9004
source: staticcheck
code: SA9004
title: Only the first constant of this group has the type you gave it
course: 09-statuspage
links:
- staticcheck: SA9004 | https://staticcheck.dev/docs/checks/#SA9004
- The Go spec: Constant declarations | https://go.dev/ref/spec#Constant_declarations
- The Go spec: Iota | https://go.dev/ref/spec#Iota
---
## What it means

In this `const` group, the first constant has an explicit type, and the next ones have their own values but no type. Those are untyped constants, not values of your type: in a variable they become `int`. Methods of your type, such as `String`, do not apply to them, and a variable declared from them has the wrong type.

## Why Go does this

The Go spec lets a constant in a group leave out its type and value: it then repeats the expression and the type of the line before. That is how a group with `iota` gives every constant the same type. But a constant that has its own value and no type is untyped. So the type carries over only when the line has no value.

## How to find it

Go to {{file}}:{{line}} and look at the lines of the group after the first one. Choose 1 fix:

- Give every constant the type: `Medium Priority = 2`.
- Use `iota`, and leave out type and value after the first line. Then every constant gets the type.

## Example

```go
const (
	Low    Priority = 1
	Medium          = 2 // an untyped int, not a Priority
)
```

Give every constant the type:

```go
const (
	Low    Priority = 1
	Medium Priority = 2
)
```

## Learn more

The course exercise below uses a constant group with iota for its own type.
