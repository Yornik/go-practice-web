---
id: staticcheck-sa6005
source: staticcheck
code: SA6005
title: Compare strings without case with strings.EqualFold
links:
- staticcheck: SA6005 | https://staticcheck.dev/docs/checks/#SA6005
- Package strings: EqualFold | https://pkg.go.dev/strings#EqualFold
---
## What it means

You convert 2 strings to lower or upper case and compare the results. That creates 2 new strings and converts them completely, even when the first characters already differ. `strings.EqualFold` compares the strings without case, 1 character at a time, creates no new strings and stops at the first difference.

## Why Go does this

`strings.ToLower` must return a new string, because strings cannot change, so each call can allocate. The docs of `strings.EqualFold` say that it compares under simple Unicode case folding, a more general form of case-insensitivity. For ASCII text the result is the same as with `ToLower`. staticcheck reports the pattern because it is slower and says less clearly what you mean.

## How to find it

Go to {{file}}:{{line}} and replace the comparison with `strings.EqualFold(a, b)`. When you compare a value with 1 fixed word in a known case, `EqualFold` works there too. When you need the lower-case form later anyway, for example as a map key, converting once and keeping the result is fine.

## Example

```go
return strings.ToLower(a) == strings.ToLower(b) // 2 new strings
```

Compare without case in 1 call:

```go
return strings.EqualFold(a, b)
```

## Learn more
