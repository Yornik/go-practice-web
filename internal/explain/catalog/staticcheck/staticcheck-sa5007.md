---
id: staticcheck-sa5007
source: staticcheck
code: SA5007
title: The function calls itself on every path, so it never returns
links:
- staticcheck: SA5007 | https://staticcheck.dev/docs/checks/#SA5007
- Package fmt: Stringer | https://pkg.go.dev/fmt#Stringer
---
## What it means

The function at {{file}}:{{line}} calls itself, and no condition stops that. Each call makes the next call, until the stack of the goroutine is full. The program then stops with `fatal error: stack overflow`. The usual cause is a method that should call a field, another method or the underlying value, and calls itself by mistake.

## Why Go does this

Recursion is legal, so the compiler accepts the call. A goroutine's stack grows as needed, up to a limit of 1 GB on 64-bit systems, and the runtime stops the program when it reaches that limit. staticcheck reports a function that reaches a call to itself on every path, because such a function can never return.

## How to find it

Go to the call at {{file}}:{{line}} and ask what you meant to call:

- The underlying value of your type: convert the receiver, as in `float64(c)`.
- A field or an embedded type with a method of the same name: name it, as in `c.inner.Close()`.
- Real recursion: add the base case, a condition that returns without calling again.

## Example

```go
type Celsius float64

func (c Celsius) Float() float64 {
	return c.Float() // calls itself forever
}
```

Convert the receiver to its underlying type:

```go
func (c Celsius) Float() float64 {
	return float64(c)
}
```

## Learn more
