---
id: vet-shift
source: vet
analyzer: shift
match: ^(?P<expr>.+) \((?:\w+ )*(?P<bits>\d+) bits\) too small for shift of (?P<shift>\d+)$
title: {{expr}} has {{bits}} bits, so a shift by {{shift}} pushes every bit out
course: 49-diskbuffer
links:
- The Go spec: Arithmetic operators | https://go.dev/ref/spec#Arithmetic_operators
- The Go spec: Integer overflow | https://go.dev/ref/spec#Integer_overflow
- Package encoding/binary | https://pkg.go.dev/encoding/binary
---
## What it means

`{{expr}}` has a type of {{bits}} bits, and you shift it by {{shift}}. Every bit moves out of the value, so nothing of it is left: a left shift gives 0. A conversion around the expression does not help, because Go shifts first, in the type of `{{expr}}`, and converts the result afterwards.

## Why Go does this

A shift has the type of its left operand. The Go spec defines a shift as shifting n times by 1, and for unsigned integers it computes `<<` modulo 2 to the power of the bit width: the high bits are dropped. The compiler rejects an overflowing shift only for constants. For a variable it computes the result, so `go vet` reports shifts that can only lose all bits.

## How to find it

Go to {{file}}:{{line}} and find the type of `{{expr}}`. An element of a `[]byte` is a `byte`, which has 8 bits. Convert the operand to the wider type before you shift, not the result after. For byte orders, package encoding/binary does the whole job: `binary.BigEndian.Uint16(b)` reads 2 bytes as a number.

## Example

```go
return uint16(b[0]<<8 | b[1]) // b[0]<<8 is a byte shift: always 0
```

Convert each byte first, then shift:

```go
return uint16(b[0])<<8 | uint16(b[1])
```

## Learn more

The course exercise below practises a binary format with encoding/binary.
