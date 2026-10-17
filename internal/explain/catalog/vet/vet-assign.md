---
id: vet-assign
source: vet
analyzer: assign
match: ^self-assignment of (?P<expr>.+)$
title: {{expr}} is assigned to itself
links:
- The Go spec: Assignment statements | https://go.dev/ref/spec#Assignment_statements
- The Go spec: Selectors | https://go.dev/ref/spec#Selectors
- The assign check of go vet | https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/assign
---
## What it means

The statement at {{file}}:{{line}} assigns `{{expr}}` to itself. It changes nothing. Most likely you meant to assign to something else with a similar name, and that other variable or field keeps its old value.

## Why Go does this

Go allows any assignment between values of the right type, and `x = x` reads `x`, so the compiler is satisfied. `go vet` reports it, because a statement that never has an effect is almost always a typo.

## How to find it

Go to {{file}}:{{line}} and ask which variable should receive the value. The usual cause is a parameter with the same name as a field. Inside a method, a bare name refers to the parameter or a local variable, never to a field: Go has no implicit `this`. A field always needs its receiver, as in `p.size`. The same holds for a variable in an outer scope that a local variable with the same name hides.

## Example

```go
func (p *Pool) SetSize(size int) {
	size = size // the parameter, twice; the field stays the same
}
```

Name the field through the receiver:

```go
func (p *Pool) SetSize(size int) {
	p.size = size
}
```

## Learn more
