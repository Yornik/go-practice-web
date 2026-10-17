---
id: staticcheck-s1005
source: staticcheck
code: S1005
title: The assignment to the blank identifier is not needed
links:
- staticcheck: S1005 | https://staticcheck.dev/docs/checks/#S1005
- The Go spec: For statements with range clause | https://go.dev/ref/spec#For_range
- Effective Go: The blank identifier | https://go.dev/doc/effective_go#blank
---
## What it means

You assign a value to `_` where you can leave the value out. In a `range` loop, `for k, _ := range m` and `for k := range m` do the same. A receive works the same way: `_ = <-ch` is `<-ch`. The code works, but the `_` is noise.

## Why Go does this

The Go spec makes the second iteration variable of a `range` clause optional: with 1 variable you get the index or the key. You need `_` only for a value that comes before a value you use, as in `for _, v := range s`. staticcheck suggests the shorter form because it is the usual Go style.

## How to find it

Go to {{file}}:{{line}} and remove the `, _` or the `_ =`. Keep `_` where it holds a place before a value that you use, or where it discards a result on purpose, such as an error that you decided to ignore. That last case deserves a comment that says why.

## Example

```go
for k, _ := range m {
	keys = append(keys, k)
}
```

Leave out the value that you do not use:

```go
for k := range m {
	keys = append(keys, k)
}
```

## Learn more
