---
id: staticcheck-s1
source: staticcheck
code: S1
title: staticcheck suggests a simpler way to write this ({{code}})
links:
- staticcheck: S1, code simplifications | https://staticcheck.dev/docs/checks/#S1
- Effective Go | https://go.dev/doc/effective_go
---
## What it means

The code at {{file}}:{{line}} works. staticcheck knows a shorter form that does the same and is the usual Go style, and the message shows it. staticcheck groups these checks under S1: code simplifications. This is check {{code}}. You do not have to change it to pass the tests, but the suggestion often teaches a function or idiom worth knowing.

## Why Go does this

Go code is read far more often than it is written. The simplifications point to helpers in the standard library and to forms that every Go programmer recognizes, such as `time.Since` for the time since a moment. Code that looks familiar is easier to review and harder to get wrong.

## How to find it

Read the suggestion in the message and check that it means the same in your code. Then apply it. If the shorter form does not fit, for example because you need the longer form to add a step, keep yours. The staticcheck page explains every check with an example: search it for {{code}}.

## Example

```go
age := time.Now().Sub(start)
```

Use the helper that says what you compute:

```go
age := time.Since(start)
```

## Learn more
