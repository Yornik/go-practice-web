---
id: staticcheck-s1017
source: staticcheck
code: S1017
match: ^should replace this if statement with an unconditional (?P<func>\S+)$
title: {{func}} already checks for the prefix, so the if is not needed
links:
- staticcheck: S1017 | https://staticcheck.dev/docs/checks/#S1017
- Package strings: TrimPrefix | https://pkg.go.dev/strings#TrimPrefix
- Package strings: CutPrefix | https://pkg.go.dev/strings#CutPrefix
---
## What it means

The `if` checks for a prefix with `strings.HasPrefix` and then slices it off. `{{func}}` does both steps: it removes the prefix when it is there and returns the string unchanged when it is not. The manual version also repeats the length of the prefix as a number in the slice expression, which is easy to get wrong when the prefix changes.

## Why Go does this

Package strings offers the common operations as single functions, and they handle the edge cases, such as an empty string. The compiler accepts the manual version. staticcheck recognises the pattern and suggests the function.

## How to find it

Go to {{file}}:{{line}} and replace the `if` with 1 call to `{{func}}`. When the code also needs to know whether the prefix was there, use `strings.CutPrefix`: it returns the rest of the string and a boolean. For the end of a string, `strings.TrimSuffix` and `strings.CutSuffix` do the same.

## Example

```go
if strings.HasPrefix(tag, "v") {
	tag = tag[1:]
}
```

Let TrimPrefix check and cut:

```go
tag = strings.TrimPrefix(tag, "v")
```

## Learn more
