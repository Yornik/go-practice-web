---
id: vet-unusedresult
source: vet
analyzer: unusedresult
match: ^result of (?P<call>.+) call not used$
title: The result of {{call}} is thrown away
links:
- Package slices | https://pkg.go.dev/slices
- The Go blog: Arrays, slices (and strings): The mechanics of append | https://go.dev/blog/slices
- The unusedresult check of go vet | https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/unusedresult
---
## What it means

You call `{{call}}` as a statement and ignore what it returns. For this function the result is the whole point: it computes a new value and changes nothing else that you can rely on. The call does useless work, and the next lines still see the old value.

## Why Go does this

Some functions only return a value: `fmt.Sprintf` and `errors.New` build a string or an error, and `slices.Clone` makes a copy. Others, such as `slices.Delete`, `slices.Insert` and `slices.Compact`, return the slice with its new length. They may change the elements of the old slice, but the caller's slice header keeps its old length. `slices.Delete` even sets the elements at the end to their zero value. The compiler accepts a call that drops its result, so `go vet` keeps a list of functions whose result you must use.

## How to find it

Go to {{file}}:{{line}}. Read the signature of `{{call}}` in its docs: what does it return? Assign the result, usually to the same variable, or return it. For an error that you built and dropped, decide whether to return it.

## Example

```go
slices.Delete(ids, i, i+1) // ids keeps its length; its last element becomes ""
return ids
```

Use the slice that Delete returns:

```go
ids = slices.Delete(ids, i, i+1)
return ids
```

## Learn more
