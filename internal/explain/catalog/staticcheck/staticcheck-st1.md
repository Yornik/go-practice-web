---
id: staticcheck-st1
source: staticcheck
code: ST1
title: staticcheck suggests a change of style ({{code}})
links:
- staticcheck: ST1, stylistic issues | https://staticcheck.dev/docs/checks/#ST1
- Go Code Review Comments | https://go.dev/wiki/CodeReviewComments
- Effective Go: Names | https://go.dev/doc/effective_go#names
---
## What it means

The code at {{file}}:{{line}} works, but it breaks a convention that most Go code follows: how to name things, how to write error text, or in which order to return results. staticcheck groups these checks under ST1: stylistic issues. This is check {{code}}. Code that follows the conventions reads like the standard library, which makes it easier for others to review.

## Why Go does this

The conventions of Go are written down in Effective Go and in the Go Code Review Comments. The compiler does not check them. staticcheck checks some of them, so that you learn them while you write.

## How to find it

Read the message: it names the convention and often the fix. Look the rule up on the staticcheck page, searching for {{code}}, or in the Code Review Comments. When an exercise README or a test prescribes a name or a message, follow the README: the tests compare it.

## Example

```go
func Wait(timeoutSecs time.Duration) // a Duration has its unit built in
```

Name the value after what it is:

```go
func Wait(timeout time.Duration)
```

## Learn more
