---
id: staticcheck-sa1029
source: staticcheck
code: SA1029
title: A context key of a built-in type can collide with other packages
course: 16-middleware
links:
- staticcheck: SA1029 | https://staticcheck.dev/docs/checks/#SA1029
- Package context: WithValue | https://pkg.go.dev/context#WithValue
- The Go blog: Go Concurrency Patterns: Context | https://go.dev/blog/context
---
## What it means

You store a value in a context with `context.WithValue` and a key of a built-in type, such as a `string`. Every other package that uses the same string as a key reads or overwrites your value. A request passes through many packages, so 2 of them choosing the key `"user"` is likely, and nothing warns either of them.

## Why Go does this

`Value` compares keys by their type and their value. 2 keys of type `string` with the same text are equal, wherever they come from. The docs of `context.WithValue` say that a key should not be of type `string` or any other built-in type, and that you should define your own type for keys. A key of an unexported type of your package can never equal a key of another package.

## How to find it

Go to {{file}}:{{line}} and look at the second argument of `WithValue`. Declare an unexported type in your package for the key, and use a value of that type. An empty struct type costs no memory. Keep the key unexported, and give callers small functions to set and read the value, so that only your package touches the key.

## Example

```go
ctx = context.WithValue(ctx, "user", user) // any package can use "user"
```

Use a key of your own, unexported type:

```go
type userKey struct{}

ctx = context.WithValue(ctx, userKey{}, user)
```

## Learn more

The course exercise below practises values in a context with an unexported key type.
