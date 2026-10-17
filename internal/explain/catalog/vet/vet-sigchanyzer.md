---
id: vet-sigchanyzer
source: vet
analyzer: sigchanyzer
match: ^misuse of unbuffered os\.Signal channel as argument to signal\.Notify$
title: signal.Notify needs a buffered channel, or it can drop the signal
course: 15-healthcheck
links:
- Package os/signal: Notify | https://pkg.go.dev/os/signal#Notify
- Package os/signal: NotifyContext | https://pkg.go.dev/os/signal#NotifyContext
- The sigchanyzer check of go vet | https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/sigchanyzer
---
## What it means

You pass an unbuffered channel to `signal.Notify`. Package signal never blocks when it delivers a signal: when no goroutine is receiving at that moment, the signal is dropped. With an unbuffered channel that happens whenever your goroutine is busy with something else. A Ctrl-C, or the SIGTERM that Kubernetes sends before it stops a pod, can then get lost, and the process does not shut down cleanly.

## Why Go does this

A signal arrives at any moment, and the runtime cannot wait for your code. The docs of `signal.Notify` say that the caller must give the channel enough buffer space, and that a buffer of 1 is enough for a channel that waits for 1 signal. The compiler does not know this rule, so `go vet` checks the channel.

## How to find it

Go to {{file}}:{{line}} and find where the channel is made. Give it a buffer: `make(chan os.Signal, 1)`. For a graceful shutdown, `signal.NotifyContext` is often simpler: it returns a context that is canceled when the signal arrives, and it manages the channel for you.

## Example

```go
c := make(chan os.Signal) // unbuffered: a signal can be lost
signal.Notify(c, os.Interrupt)
<-c
```

Give the channel room for 1 signal:

```go
c := make(chan os.Signal, 1)
signal.Notify(c, os.Interrupt)
<-c
```

## Learn more

The course exercise below practises a graceful shutdown on a signal.
