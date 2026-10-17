---
id: test-waited-for
source: test
match: ^timed out after (?P<limit>(?:\d+(?:\.\d+)?(?:ns|µs|us|ms|s|m|h))+) waiting (?:until|for) (?P<what>[^\n]+)$
title: The test waited {{limit}} for this to happen: {{what}}
course: 12-workerpool
links:
- The Go spec: Go statements | https://go.dev/ref/spec#Go_statements
- Package sync: WaitGroup | https://pkg.go.dev/sync#WaitGroup
- Effective Go: Concurrency | https://go.dev/doc/effective_go#concurrency
---
## What it means

The test started your code and then checked, again and again, whether this was true: "{{what}}". After {{limit}} it still was not, so the test gave up at {{file}}:{{line}}. Your code did not reach a state that the spec asks for, such as several calls running at the same time, a callback being called, or a channel being closed.

## Why Go does this

Concurrent behaviour cannot be checked with 1 look at a return value. So the course tests poll: a function that the test gave your code counts what happens, and the test waits until the count is right. {{limit}} is far longer than the work needs, so a slow machine is not the cause. Something in your code keeps the state from ever being reached.

## How to find it

Read the test around {{file}}:{{line}}: which counter or channel does the condition look at, and which call of your code should change it? Then follow that call:

- Does your code wait for each piece of work before it starts the next one? A `wg.Wait()` or a receive inside the loop that starts goroutines makes them run 1 after another.
- Does it hold a lock while it runs the work, so that only 1 can run at a time?
- Is it blocked earlier, before it even starts the work?
- Does it count a limit wrong, by 1 or more?

Print a line when each piece of work starts and ends, and run only this test.

## Example

```go
for _, f := range fns {
	wg.Add(1)
	go func() {
		defer wg.Done()
		f()
	}()
	wg.Wait() // waits for this f before the next one starts
}
```

Start all of them first, then wait once:

```go
for _, f := range fns {
	wg.Add(1)
	go func() {
		defer wg.Done()
		f()
	}()
}
wg.Wait()
```

## Learn more
