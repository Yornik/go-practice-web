---
id: vet-stringintconv
source: vet
analyzer: stringintconv
match: ^conversion from (?P<from>.+) to (?P<to>\S+) yields a string of one rune, not a string of digits$
title: Converting {{from}} to {{to}} gives 1 character, not the digits of the number
links:
- The Go spec: Conversions to and from a string type | https://go.dev/ref/spec#Conversions_to_and_from_a_string_type
- Package strconv: Itoa | https://pkg.go.dev/strconv#Itoa
- The stringintconv check of go vet | https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/stringintconv
---
## What it means

The conversion `string(x)` of an integer does not write the number in digits. It treats the number as a Unicode code point and gives the character with that code: `string(65)` is `"A"`, and `string(80)` is `"P"`. Where you expected `80`, your string contains `P`.

## Why Go does this

The Go spec keeps this conversion for historical reasons: an integer converted to a string yields the UTF-8 encoding of that code point. It is almost never what you mean, so `go vet` reports it. `go test` runs this check before the tests, so the conversion stops the test run.

## How to find it

Go to {{file}}:{{line}} and decide what you want:

- The digits of a number: `strconv.Itoa(n)` for an `int`, `strconv.FormatInt` for an `int64`, or `fmt.Sprint(n)` for any number.
- A character with that code: convert to `rune` first, as in `string(rune(r))`. That tells the reader, and `go vet`, that you mean it.

## Example

```go
return "port " + string(port) // "port P" for 80
```

Format the number as digits:

```go
return "port " + strconv.Itoa(port)
```

## Learn more
