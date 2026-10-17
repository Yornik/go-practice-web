---
id: staticcheck-st1006
source: staticcheck
code: ST1006
title: Name the receiver after its type, not this or self
links:
- staticcheck: ST1006 | https://staticcheck.dev/docs/checks/#ST1006
- Go Code Review Comments: Receiver Names | https://go.dev/wiki/CodeReviewComments#receiver-names
- The Go spec: Method declarations | https://go.dev/ref/spec#Method_declarations
---
## What it means

The receiver of this method is named `this`, `self` or another generic name. In Go, the receiver is a parameter like any other, and its name should say what it is. A short name derived from the type, such as `b` for a `Batch`, is the common style.

## Why Go does this

The Go Code Review Comments say that the receiver name should reflect its identity, and that 1 or 2 letters from the type often suffice. Names such as `this` and `self` come from object-oriented languages, where the receiver has a special meaning. In Go it does not: a method is a function whose first parameter is written before its name.

## How to find it

Go to {{file}}:{{line}} and rename the receiver to a short name based on the type. Use the same name in every method of that type, so that a reader can move between methods without relearning it. Rename all uses inside the method body as well.

## Example

```go
func (self *Batch) Len() int { return len(self.items) }
```

Use a short name based on the type:

```go
func (b *Batch) Len() int { return len(b.items) }
```

## Learn more
