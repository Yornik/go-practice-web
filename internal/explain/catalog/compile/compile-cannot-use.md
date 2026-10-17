---
id: compile-cannot-use
source: compile
match: ^cannot use (?P<expr>.+) \((?P<what>.+)\) as (?P<want>.+) value in (?P<context>[^:()\n]+)$
title: {{expr}} cannot be used as {{want}} here
links:
- The Go spec: Assignability | https://go.dev/ref/spec#Assignability
- The Go spec: Conversions | https://go.dev/ref/spec#Conversions
- Package strconv | https://pkg.go.dev/strconv
---
## What it means

At {{file}}:{{line}}, the {{context}} needs a value of type `{{want}}`, but `{{expr}}` ({{what}}) has another type. Go does not convert it for you.

## Why Go does this

A value can go into a variable, a parameter or a result only when it is assignable to that type. In practice that means the 2 types are identical, the target is an interface that the value implements, or the value is `nil` or an untyped constant that fits. Go has no implicit conversions between numeric types, between a pointer and its value, or between a named type such as `time.Duration` and the type it is built on. Each such conversion can lose information or change meaning, so you write it down.

## How to find it

Compare the 2 types in the message, the type of `{{expr}}` and `{{want}}`:

- `T` and `*T`: take the address with `&x`, or follow the pointer with `*p`. A function that changes a struct usually takes `*T`.
- 2 numeric types, such as `int` and `int64` or `int` and `time.Duration`: convert with `int64(n)` or `time.Duration(n)`.
- A number and a `string`: a conversion does not do it; use `strconv.Itoa`, `strconv.Atoi` or `strconv.ParseFloat`.
- `[]byte` and `string`: convert with `string(b)` or `[]byte(s)`.
- In a test file, the test passes what the exercise API declares. Fix the type in your code.

## Example

```go
func StatusText(code int) string {
	if code == 200 {
		return "ok"
	}
	return code // cannot use code (variable of type int) as string value in return statement
}
```

Turn the number into text with strconv:

```go
	return strconv.Itoa(code)
```

## Learn more
