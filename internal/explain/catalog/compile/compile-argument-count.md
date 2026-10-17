---
id: compile-argument-count
source: compile
match: ^(?P<which>too many|not enough) arguments in call to (?P<func>[^\n]+)\n\s*have (?P<have>\([^\n]*\))\n\s*want (?P<want>\([^\n]*\))$
title: Wrong number of arguments in the call to {{func}}
links:
- The Go spec: Calls | https://go.dev/ref/spec#Calls
- The Go spec: Passing arguments to ... parameters | https://go.dev/ref/spec#Passing_arguments_to_..._parameters
- Go FAQ: Why does Go not support overloading? | https://go.dev/doc/faq#overloading
---
## What it means

The call at {{file}}:{{line}} passes a different number of arguments than `{{func}}` declares. The 2 lines under the message compare them: `have {{have}}` lists the types of the arguments you passed, `want {{want}}` lists the parameters. `number` in the `have` line stands for an untyped numeric constant such as `3`.

## Why Go does this

Go has no default values for parameters and no overloading, so each function has exactly 1 signature, and every call passes 1 argument per parameter. The only exception is a final variadic parameter `...T`, which takes 0 or more values.

## How to find it

Compare `have` and `want` position by position and find the first one that differs.

- Did the signature change? When you add a parameter, every call needs it, and the error moves to each of them.
- Did you forget an argument that is easy to overlook, such as a `context.Context`, a clock or a limit?
- When the call is in a test file, the test calls the API from the exercise README. Change your function to that signature, not the test.
- Look up the signature of a library function with `go doc`, for example `go doc strings.Cut`.

## Example

```go
func Scale(n int, unit time.Duration) time.Duration { ... }

d := Scale(3)
```

```text
not enough arguments in call to Scale
	have (number)
	want (int, time.Duration)
```

Pass 1 value for each parameter:

```go
d := Scale(3, time.Millisecond)
```

## Learn more
