---
id: runtime-reflect-wrong-kind
source: runtime
match: ^panic: reflect: call of reflect\.Value\.(?P<method>\w+) on (?P<kind>bool|u?int(?:8|16|32|64|ptr)?|float32|float64|complex64|complex128|array|chan|func|interface|map|ptr|slice|string|struct|unsafe\.Pointer) Value$
match: ^panic: reflect: (?P<method>\w+) of non-struct type (?P<kind>\S+)$
title: reflect: {{method}} does not work on a {{kind}}
course: 31-envconfig
links:
- The Go blog: The Laws of Reflection | https://go.dev/blog/laws-of-reflection#the-first-law-of-reflection
- Package reflect: Value | https://pkg.go.dev/reflect#Value
- Package reflect: Kind | https://pkg.go.dev/reflect#Kind
---
## What it means

At {{file}}:{{line}}, `{{func}}` called the reflect method `{{method}}` on a value or a type whose kind is `{{kind}}`, and that method does not apply to that kind. `Int` and `SetInt` work only on the int kinds, `SetString` only on strings, `NumField` and `Field` only on structs.

## Why Go does this

`reflect.Value` and `reflect.Type` represent values of every type, so the compiler cannot check which of their methods make sense for your value. The docs of package reflect say it plainly: not all methods apply to all kinds, use `Kind` to find out the kind before you call a kind-specific method, and a wrong call panics. Each method lists the kinds it accepts.

## How to find it

Print `v.Kind()` and `v.Type()` right before the line, and compare with what the method needs:

- `ptr`, or a type that starts with `*`: you hold a pointer to the struct, not the struct. Call `Elem()` first.
- A struct whose fields have different types, walked in a loop: the loop calls the same method on every field. Check each field's kind before the call, with a `switch` on `Kind()` or a method such as `CanInt`.
- `interface`: the value is stored in an interface. `Elem()` gives the value inside.

## Example

```go
v := reflect.ValueOf(stats)
for i := range v.NumField() {
	total += v.Field(i).Int() // panics at the first string field
}
```

Ask before you call:

```go
	if f := v.Field(i); f.CanInt() {
		total += f.Int()
	}
```

## Learn more

The course exercise below walks the fields of a struct with reflection.
