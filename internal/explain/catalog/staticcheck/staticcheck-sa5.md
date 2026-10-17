---
id: staticcheck-sa5
source: staticcheck
code: SA5
title: staticcheck found code that cannot work as written ({{code}})
links:
- staticcheck: SA5, correctness issues | https://staticcheck.dev/docs/checks/#SA5
- The Go Memory Model | https://go.dev/ref/mem
---
## What it means

The code at {{file}}:{{line}} compiles, but it cannot work correctly: it panics, never returns, spins forever or loses data. The message says what staticcheck found. staticcheck groups these checks under SA5: correctness issues. This is check {{code}}.

## Why Go does this

The compiler accepts every program that is valid Go. staticcheck knows patterns that are wrong in almost every program, such as an empty loop that waits for a variable to change. In that case the loop burns a whole CPU core. Without synchronization, the Go memory model does not even promise that the loop ever sees the change made by another goroutine.

## How to find it

Go to {{file}}:{{line}}. Write a small test that takes this path, and watch what happens: a panic, a hang, or a wrong result. Then fix the cause that the message names. The staticcheck page explains every check with an example: search it for {{code}}.

## Example

```go
for !*ready { // spins, and may never see the change
}
```

Wait on a channel that the other goroutine closes:

```go
<-ready // ready is a chan struct{}
```

## Learn more
