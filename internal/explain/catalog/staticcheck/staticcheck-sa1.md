---
id: staticcheck-sa1
source: staticcheck
code: SA1
title: staticcheck found a misuse of the standard library ({{code}})
links:
- staticcheck: SA1, misuses of the standard library | https://staticcheck.dev/docs/checks/#SA1
- The Go standard library | https://pkg.go.dev/std
---
## What it means

The call at {{file}}:{{line}} passes an argument that the function cannot use as intended. The code compiles, but the call fails when it runs, or does something other than its docs promise. The message names the argument and the problem. staticcheck groups these checks under SA1: misuses of the standard library. This is check {{code}}.

## Why Go does this

Many functions take arguments with rules that the type system cannot express: a regular expression, a time layout, a template, a number in a valid range, or a pointer where a value would be lost. The compiler checks only the types. staticcheck knows the rules of these functions and checks the arguments that it can evaluate, such as constants.

## How to find it

Read the message, then the docs of the function that is called at {{file}}:{{line}}. Find the rule that the argument breaks. The staticcheck page lists every check with an explanation and an example: search it for {{code}}. Fix the argument, and then test the call with input that takes the path through it.

## Example

```go
var level = regexp.MustCompile(`level=(\w+`) // panics: missing closing )
```

Make the argument follow the rules of the function:

```go
var level = regexp.MustCompile(`level=(\w+)`)
```

## Learn more
