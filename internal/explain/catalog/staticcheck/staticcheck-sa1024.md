---
id: staticcheck-sa1024
source: staticcheck
code: SA1024
title: The second argument of Trim, TrimLeft or TrimRight is a set of characters, not a prefix
links:
- staticcheck: SA1024 | https://staticcheck.dev/docs/checks/#SA1024
- Package strings: TrimLeft | https://pkg.go.dev/strings#TrimLeft
- Package strings: TrimPrefix | https://pkg.go.dev/strings#TrimPrefix
---
## What it means

`strings.TrimLeft`, `TrimRight` and `Trim` take a cutset: a set of characters. They remove every leading or trailing character that is in the set, in any order and any number of times. Your cutset holds a character twice, which suggests that you meant a whole string. `strings.TrimLeft("https://status.example.com", "https://")` returns `atus.example.com`: the `s` and `t` of the host name are in the set too.

## Why Go does this

The docs of `strings.TrimLeft` say that it removes all leading Unicode code points contained in the cutset, and that you should use `TrimPrefix` to remove a prefix. Both arguments are strings, so the compiler cannot tell what you meant. The bug hides in tests whose input happens not to start with a letter from the set.

## How to find it

Go to {{file}}:{{line}} and ask whether you want to remove a set of characters or 1 exact string:

- 1 exact string at the start or end: `strings.TrimPrefix` or `strings.TrimSuffix`.
- 1 exact string, and you need to know whether it was there: `strings.CutPrefix`.
- A set of characters, such as spaces and tabs: keep the Trim function, and list each character once.

## Example

```go
host := strings.TrimLeft(url, "https://") // "atus.example.com" for https://status.example.com
```

Remove the exact prefix:

```go
host := strings.TrimPrefix(url, "https://")
```

## Learn more
