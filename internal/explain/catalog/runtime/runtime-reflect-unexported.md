---
id: runtime-reflect-unexported
source: runtime
match: ^panic: reflect: reflect\.Value\.(?P<method>\w+) using value obtained using unexported field$
title: reflect: {{method}} cannot change an unexported field
course: 06-envconfig
links:
- The Go spec: Exported identifiers | https://go.dev/ref/spec#Exported_identifiers
- Package reflect: StructField.IsExported | https://pkg.go.dev/reflect#StructField.IsExported
- Package reflect: Value.CanSet | https://pkg.go.dev/reflect#Value.CanSet
---
## What it means

At {{file}}:{{line}}, `{{func}}` called `{{method}}` on a struct field whose name starts with a lower-case letter, or on a value reached through such a field. Reflection can see unexported fields, but it never lets code change them.

## Why Go does this

An unexported name belongs to its package; the spec calls only names with an upper-case first letter exported. Reflection keeps that promise at run time: a Value obtained through an unexported field is read-only, even in the same package, and `CanSet` returns false for it. Without that rule, any package could change the private state of any other through reflect.

## How to find it

This usually happens in a loop over all fields of a struct, or after a lookup by name. Look at the code that reaches line {{line}}, and decide what your function should do with unexported fields; the spec of the function may say. Then check each field before you change it:

- `t.Field(i).IsExported()` on the `reflect.Type` tells you from the name.
- `v.Field(i).CanSet()` on the `reflect.Value` also covers values that are not addressable.

Print the field name with `t.Field(i).Name` when you are not sure which field it is.

## Example

```go
f := reflect.ValueOf(&opts).Elem().FieldByName(name) // name is "token", an unexported field
f.SetString(value) // panics: FieldByName finds it, but it is read-only
```

Check before you change it, and report the problem:

```go
if !f.CanSet() {
	return fmt.Errorf("option %q cannot be set", name)
}
f.SetString(value)
```

## Learn more
