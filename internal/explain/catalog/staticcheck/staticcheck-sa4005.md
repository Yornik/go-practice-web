---
id: staticcheck-sa4005
source: staticcheck
code: SA4005
match: ^ineffective assignment to field (?P<field>\S+)$
title: The assignment to {{field}} changes a copy and is lost
course: 30-slo
links:
- staticcheck: SA4005 | https://staticcheck.dev/docs/checks/#SA4005
- Effective Go: Pointers vs. Values | https://go.dev/doc/effective_go#pointers_vs_values
- The Go spec: Method declarations | https://go.dev/ref/spec#Method_declarations
---
## What it means

The method has a value receiver, so it works on a copy of the struct. The assignment to `{{field}}` changes that copy, and the copy is gone when the method returns. The caller's value never changes. A test that calls the method and then checks the field sees the old value.

## Why Go does this

A receiver is a parameter, and Go passes parameters by value. A method with a value receiver gets its own copy. Only a pointer receiver lets a method change the caller's value. The compiler accepts the assignment, because changing a local copy is legal. staticcheck reports it, because the new value is never read.

## How to find it

Go to {{file}}:{{line}} and decide what the method should do:

- Change the value in place: use a pointer receiver, as in `func (p *Point) Move(...)`. Then use pointer receivers for the other methods of the type too.
- Leave the value alone and return a changed copy, as `time.Time.Add` does: return the new value and let the caller assign it.

## Example

```go
func (p Point) Move(dx, dy int) {
	p.X += dx // changes the copy
	p.Y += dy
}
```

Use a pointer receiver, or return the moved point:

```go
func (p *Point) Move(dx, dy int) {
	p.X += dx
	p.Y += dy
}
```

## Learn more

The course exercise below practises value receivers on purpose, with methods that return results.
