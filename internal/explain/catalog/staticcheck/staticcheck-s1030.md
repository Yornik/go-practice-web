---
id: staticcheck-s1030
source: staticcheck
code: S1030
match: ^should use (?P<want>\S+\.(?:String|Bytes)\(\)) instead of (?P<have>.+)$
title: Use {{want}} instead of {{have}}
links:
- staticcheck: S1030 | https://staticcheck.dev/docs/checks/#S1030
- Package bytes: Buffer | https://pkg.go.dev/bytes#Buffer
- Package strings: Builder | https://pkg.go.dev/strings#Builder
---
## What it means

A `bytes.Buffer` has a `String` method and a `Bytes` method. You get 1 of them through the other and a conversion, as in `{{have}}`. `{{want}}` returns the same result in 1 call and says directly what you want.

## Why Go does this

Converting between `[]byte` and `string` copies the data. `Buffer.String` returns the contents as a string, and `Buffer.Bytes` returns them as a slice that shares the buffer's memory. The compiler accepts the conversion. staticcheck suggests the method that gives the type you need.

## How to find it

Go to {{file}}:{{line}} and replace `{{have}}` with `{{want}}`. Then ask whether you need a `bytes.Buffer` at all. When you only build a string, `strings.Builder` is made for that, and it keeps copying to a minimum.

## Example

```go
return string(buf.Bytes())
```

Ask the buffer for a string:

```go
return buf.String()
```

## Learn more
