---
id: runtime-reflect-unaddressable
source: runtime
match: ^panic: reflect: reflect\.Value\.(?P<method>\w+) using unaddressable value$
title: reflect: {{method}} cannot change a copy
course: 06-envconfig
links:
- The Go blog: The Laws of Reflection | https://go.dev/blog/laws-of-reflection#the-third-law-of-reflection
- Package reflect: Value.CanSet | https://pkg.go.dev/reflect#Value.CanSet
- Package reflect: Value.Elem | https://pkg.go.dev/reflect#Value.Elem
---
## What it means

At {{file}}:{{line}}, `{{func}}` called `{{method}}` to change a value through reflection, but the `reflect.Value` holds a copy, not your variable. A change to the copy would be lost at once, so reflect refuses and panics.

## Why Go does this

`reflect.ValueOf(x)` receives a copy of `x`, like any function argument. A Value can change memory only when it is addressable: it came from a pointer through `Elem()`, or it is a field or an element of such a Value. The Laws of Reflection calls this settability, and `CanSet` reports it. Map elements are never addressable; `SetMapIndex` changes a map.

## How to find it

Follow the Value on line {{line}} back to its `reflect.ValueOf` call:

- Did a struct go in by value, for example a parameter of type `Config` instead of `*Config`? Pass a pointer, and start with `reflect.ValueOf(p).Elem()`.
- Did the pointer go in, but the code forgot `Elem()`? Then the Value is the pointer itself, and a pointer value is not addressable either.
- Is the value a map element from `MapIndex`? Build the new value and store it with `SetMapIndex`.

Print `v.CanSet()` and `v.CanAddr()` at each step to see where the chain loses addressability.

## Example

```go
func Reset(c Config) {
	v := reflect.ValueOf(c) // a copy of c
	v.Field(0).SetString("") // panics: unaddressable
}
```

Take a pointer, and go through `Elem`:

```go
func Reset(c *Config) {
	v := reflect.ValueOf(c).Elem() // the struct c points to
	v.Field(0).SetString("")
}
```

## Learn more
