---
id: staticcheck-sa1003
source: staticcheck
code: SA1003
match: ^value of type (?P<type>.+) cannot be used with (?P<call>\S+)$
title: {{call}} cannot encode a {{type}}, because its size is not fixed
course: 24-diskbuffer
links:
- staticcheck: SA1003 | https://staticcheck.dev/docs/checks/#SA1003
- Package encoding/binary: Write | https://pkg.go.dev/encoding/binary#Write
- The Go spec: Numeric types | https://go.dev/ref/spec#Numeric_types
---
## What it means

`{{call}}` writes values in a fixed number of bytes, and a `{{type}}` has no fixed size. The call returns an error such as `binary.Write: some values are not fixed-sized in type int`, and it writes nothing. If you do not check that error, your file or message misses the value.

## Why Go does this

A binary format must have the same layout on every machine that reads it. The docs of `binary.Write` require a fixed-size value, or a slice or pointer of fixed-size values. `int` and `uint` are 32 or 64 bits wide depending on the platform, so they do not qualify. Strings and maps have no fixed size either. The function takes `any`, so the compiler cannot reject them.

## How to find it

Go to {{file}}:{{line}}. First write the format down: how many bytes the field takes, and in which byte order. Then convert the value to the type of exactly that size, such as `uint32` or `int64`. Check that the value fits: a conversion to a smaller type drops the high bits without an error. For a single number, the methods of `binary.BigEndian` and `binary.LittleEndian`, such as `AppendUint32`, are often simpler.

## Example

```go
return binary.Write(w, binary.BigEndian, n) // n is an int
```

Choose the size of the field and convert:

```go
return binary.Write(w, binary.BigEndian, uint32(n))
```

## Learn more

The course exercise below practises a binary record format.
