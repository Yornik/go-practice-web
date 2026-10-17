---
id: staticcheck-sa6
source: staticcheck
code: SA6
title: This code does more work than it needs to ({{code}})
links:
- staticcheck: SA6, performance issues | https://staticcheck.dev/docs/checks/#SA6
- Package testing: Benchmarks | https://pkg.go.dev/testing#hdr-Benchmarks
---
## What it means

The code at {{file}}:{{line}} gives the right result, but it repeats expensive work, such as compiling the same regular expression for every line, or it allocates where it does not have to. The cost grows with the input. staticcheck groups these checks under SA6: performance issues. This is check {{code}}.

## Why Go does this

Some calls of the standard library are expensive and are meant to run once, with the result reused. The compiler does not know which calls are expensive. staticcheck knows a few common cases and points them out.

## How to find it

Go to {{file}}:{{line}} and find the work that gives the same result in every iteration or every call. Do it once, outside the loop or at package level, and reuse the result. Ask too whether a simpler function does the job: a fixed prefix needs `strings.HasPrefix`, not a regular expression. A benchmark shows the difference. The staticcheck page explains every check: search it for {{code}}.

## Example

```go
for _, l := range lines {
	if ok, _ := regexp.MatchString(`^ERROR`, l); ok { // compiles the regexp every time
		n++
	}
}
```

Use a function that needs no regular expression:

```go
for _, l := range lines {
	if strings.HasPrefix(l, "ERROR") {
		n++
	}
}
```

## Learn more
