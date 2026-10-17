---
id: compile-not-an-interface
source: compile
match: ^(?:invalid operation: )?(?P<expr>.+?) \((?P<what>.+)\) is not an interface$
title: {{expr}} is not an interface, so it has no dynamic type to test
links:
- The Go spec: Type assertions | https://go.dev/ref/spec#Type_assertions
- The Go spec: Type switches | https://go.dev/ref/spec#Type_switches
- The Go spec: Conversions | https://go.dev/ref/spec#Conversions
---
## What it means

At {{file}}:{{line}} you wrote a type assertion `x.(T)` or a type switch `switch x.(type)` on `{{expr}}`, which is a {{what}}. Both only work on an interface value. A variable of a concrete type always holds exactly that type, so there is nothing to test.

## Why Go does this

An interface value can hold values of different types, and its dynamic type is known only at run time. A type assertion asks for that dynamic type. For a concrete type such as `int` or a struct, the compiler already knows the type, and an assertion has no meaning. To turn a value of 1 type into another type, Go uses a conversion `T(x)`, which the compiler checks.

## How to find it

Ask what you want to do with `{{expr}}`:

- Change its type: use a conversion, such as `float64(n)`, or a function from `strconv` to parse or format text.
- Test the type of a value that you received as `any` or as an interface: use that interface variable, not a variable that already has a concrete type.
- Inspect an error: call `errors.As` on the `error` value.
- A defined type, such as `type Status int`, is not an interface either. Its type is fixed at compile time.

## Example

```go
func Name(v int) string {
	s, ok := v.(string) // invalid operation: v (variable of type int) is not an interface
	if !ok {
		return ""
	}
	return s
}
```

Format the number instead:

```go
func Name(v int) string {
	return strconv.Itoa(v)
}
```

## Learn more
