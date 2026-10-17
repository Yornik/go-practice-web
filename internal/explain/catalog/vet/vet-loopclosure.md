---
id: vet-loopclosure
source: vet
analyzer: loopclosure
match: ^loop variable (?P<name>\S+) captured by func literal$
title: The goroutine or defer uses the loop variable {{name}}, which all iterations share
links:
- The Go blog: Fixing For Loops in Go 1.22 | https://go.dev/blog/loopvar-preview
- The Go spec: For statements with for clause | https://go.dev/ref/spec#For_clause
- The loopclosure check of go vet | https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/loopclosure
---
## What it means

A function literal inside the loop, started with `go` or `defer`, uses the loop variable `{{name}}`. In this module there is 1 `{{name}}` for the whole loop, and every iteration overwrites it. The function runs later, often after the loop has moved on, so it sees a later value. Often all goroutines see the last element.

## Why Go does this

Before Go 1.22, a `for` loop declared its variables once and reused them in every iteration. Go 1.22 changed this: each iteration now has its own variable. The change applies per module: only a module whose `go.mod` says `go 1.22` or later gets the new rule. `go vet` reports this only in older modules, because the new rule fixes the bug. The course module says `go 1.24`, so the course code never sees it. You see it when you work in an older module.

## How to find it

Go to {{file}}:{{line}}. The function literal around that line runs after the current iteration. Then choose 1 fix:

- Raise the `go` line in `go.mod` to 1.22 or later, if the module may require that release.
- Copy the variable inside the loop body: `{{name}} := {{name}}`. Each iteration then declares a new variable.
- Pass the value as an argument to the function literal.

## Example

```go
for _, task := range tasks {
	go func() {
		task() // before Go 1.22: every goroutine may run the last task
	}()
}
```

Declare a new variable in each iteration:

```go
for _, task := range tasks {
	task := task
	go func() { task() }()
}
```

## Learn more
