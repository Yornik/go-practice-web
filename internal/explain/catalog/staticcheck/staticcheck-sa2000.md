---
id: staticcheck-sa2000
source: staticcheck
code: SA2000
match: ^should call (?P<call>.+) before starting the goroutine to avoid a race$
title: Call {{call}} before the go statement, not inside the goroutine
course: 37-workerpool
links:
- staticcheck: SA2000 | https://staticcheck.dev/docs/checks/#SA2000
- Package sync: WaitGroup.Add | https://pkg.go.dev/sync#WaitGroup.Add
- The Go Memory Model | https://go.dev/ref/mem
---
## What it means

The goroutine calls `{{call}}` on the WaitGroup that counts it. A `go` statement only starts the goroutine. Before its first line runs, the code after the loop may already be in `Wait`, see a counter of 0 and return. The function then returns while the work still runs: results go missing, or `-race` reports a data race, and only in some runs.

## Why Go does this

The docs of `WaitGroup.Add` say that an `Add` with a positive delta while the counter is 0 must happen before `Wait`, which typically means before the statement that creates the goroutine. The scheduler decides when a new goroutine runs, so the compiler cannot see the order. `go vet` reports the same mistake with the message `WaitGroup.Add called from inside new goroutine`.

## How to find it

Go to {{file}}:{{line}} and move `{{call}}` to the line right before the `go` statement. Keep `defer wg.Done()` as the first line inside the goroutine. When you know the number of goroutines in advance, you can also call `Add` once with that number before the loop.

## Example

```go
go func() {
	wg.Add(1) // may run after wg.Wait
	defer wg.Done()
	handle(item)
}()
```

Count the goroutine before you start it:

```go
wg.Add(1)
go func() {
	defer wg.Done()
	handle(item)
}()
```

## Learn more

The course exercise below practises starting goroutines and waiting for all of them.
