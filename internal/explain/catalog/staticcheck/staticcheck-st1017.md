---
id: staticcheck-st1017
source: staticcheck
code: ST1017
title: Write the variable first in a comparison
links:
- staticcheck: ST1017 | https://staticcheck.dev/docs/checks/#ST1017
- The Go spec: Assignment statements | https://go.dev/ref/spec#Assignment_statements
---
## What it means

The comparison puts the constant first, as in `"" == s`. That is called a Yoda condition. In Go the usual order is the variable first: `s == ""`. The result is the same, but the reversed order makes the reader stop.

## Why Go does this

In C, a Yoda condition protects against writing `=` instead of `==`, because an assignment to a constant does not compile. Go does not need that protection: an assignment is a statement, not an expression, so `if s = ""` never compiles. staticcheck reports the reversed order as a style issue.

## How to find it

Go to {{file}}:{{line}} and swap the 2 sides of the comparison. For `<` and `>`, also turn the operator around: `0 < n` becomes `n > 0`.

## Example

```go
return "" == s
```

Put the variable first:

```go
return s == ""
```

## Learn more
