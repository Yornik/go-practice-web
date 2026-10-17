---
id: staticcheck-sa1030
source: staticcheck
code: SA1030
title: An argument of this strconv call is out of its valid range
course: 17-promtext
links:
- staticcheck: SA1030 | https://staticcheck.dev/docs/checks/#SA1030
- Package strconv: ParseInt | https://pkg.go.dev/strconv#ParseInt
- Package strconv: ParseFloat | https://pkg.go.dev/strconv#ParseFloat
---
## What it means

The message names an argument of the strconv call that has an invalid value, such as the base or the bit size. With an invalid argument the function fails for every input: `strconv.ParseUint("80", 10, 128)` returns the error `invalid bit size 128`. Your parser then rejects valid data.

## Why Go does this

The base and the bit size are plain `int` parameters, so the compiler accepts any number. The docs of `strconv.ParseInt` say that the base must be 0 or 2 to 36, and the bit size 0 to 64. Bit sizes 0, 8, 16, 32 and 64 stand for `int`, `int8`, `int16`, `int32` and `int64`. `ParseFloat` takes 32 or 64. staticcheck checks these arguments when they are constants.

## How to find it

Go to {{file}}:{{line}} and read the docs of the function. The bit size names the type that the result must fit into. The function still returns an `int64` or a `uint64`, so convert the result to the smaller type afterwards. The bit size makes the function reject values that do not fit, with an error that wraps `strconv.ErrRange`.

## Example

```go
n, err := strconv.ParseUint(s, 10, 128) // no such bit size
```

Name the size of the target type, then convert:

```go
n, err := strconv.ParseUint(s, 10, 16)
port := uint16(n)
```

## Learn more

The course exercise below practises parsing numbers with strconv.
