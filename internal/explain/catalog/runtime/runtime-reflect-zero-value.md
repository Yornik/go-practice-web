---
id: runtime-reflect-zero-value
source: runtime
match: ^panic: reflect: call of reflect\.Value\.(?P<method>\w+) on zero Value$
title: reflect: {{method}} was called on an empty reflect.Value
course: 18-tableprint
links:
- Package reflect: Value | https://pkg.go.dev/reflect#Value
- Package reflect: Value.IsValid | https://pkg.go.dev/reflect#Value.IsValid
- Package reflect: Value.FieldByName | https://pkg.go.dev/reflect#Value.FieldByName
---
## What it means

At {{file}}:{{line}}, `{{func}}` called `{{method}}` on a `reflect.Value` that holds nothing: the zero Value. An earlier reflect call returned it to say "there is nothing here", and the code went on as if there were.

## Why Go does this

The docs of package reflect say that the zero Value represents no value: `IsValid` returns false, `Kind` returns `Invalid`, and all other methods panic. A few functions return it on purpose, and their docs say when:

- `FieldByName` when the struct has no field with that name.
- `Elem` of a nil pointer or a nil interface.
- `MapIndex` when the key is not in the map.
- `reflect.ValueOf(nil)`.

## How to find it

Follow the Value on line {{line}} back to the call that produced it, and check which of the cases above applies. Print `v.IsValid()` after each step of a chain such as `reflect.ValueOf(p).Elem().FieldByName(name)` to see where it becomes invalid. Field names are case-sensitive: `port` is not `Port`. Then decide what your function should do in that case, and check before you use the Value: `IsValid()` after a lookup, `IsNil()` before `Elem()` of a pointer.

## Example

```go
f := reflect.ValueOf(&opts).Elem().FieldByName(name)
f.SetString(value) // no field called name: f is the zero Value
```

Check the result of the lookup:

```go
f := reflect.ValueOf(&opts).Elem().FieldByName(name)
if !f.IsValid() {
	return fmt.Errorf("no option %q", name)
}
```

## Learn more
