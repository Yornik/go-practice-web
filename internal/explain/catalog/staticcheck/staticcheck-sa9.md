---
id: staticcheck-sa9
source: staticcheck
code: SA9
title: This construct is valid Go but probably wrong ({{code}})
links:
- staticcheck: SA9, dubious code constructs | https://staticcheck.dev/docs/checks/#SA9
- The Go spec: Defer statements | https://go.dev/ref/spec#Defer_statements
---
## What it means

The code at {{file}}:{{line}} compiles, and it may even pass a test, but the pattern usually hides a mistake. The message says what looks wrong. staticcheck groups these checks under SA9: dubious code constructs that have a high probability of being wrong. This is check {{code}}.

## Why Go does this

Some constructs are legal but behave differently from what they seem to say, such as a `defer` inside a loop. A deferred call runs when the function returns, not at the end of the iteration. staticcheck reports these cases, because the code that you see and the code that runs do different things.

## How to find it

Go to {{file}}:{{line}} and ask what you expect the line to do, and when. Then check the rule of the language or the function. If the code is right after all, add a comment that says why. The staticcheck page explains every check with an example: search it for {{code}}.

## Example

```go
for p := range paths { // a channel
	f, err := os.Open(p)
	if err != nil {
		continue
	}
	defer f.Close() // runs only when the function returns
	process(f)
}
```

Give each iteration its own function, so that its defer runs at the end of the iteration:

```go
for p := range paths {
	processFile(p) // opens, defers the close and processes
}
```

## Learn more
