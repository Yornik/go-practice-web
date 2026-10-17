---
id: compile-unknown-field
source: compile
match: ^unknown field (?P<name>\S+) in struct literal of type (?P<type>[^,\n]+)(?:, but does have (?:unexported )?(?P<have>\S+))?$
title: The struct {{type}} has no field {{name}}
links:
- The Go spec: Composite literals | https://go.dev/ref/spec#Composite_literals
- The Go spec: Struct types | https://go.dev/ref/spec#Struct_types
- The Go spec: Exported identifiers | https://go.dev/ref/spec#Exported_identifiers
---
## What it means

The struct literal at {{file}}:{{line}} sets a field `{{name}}`, but the type `{{type}}` has no field with that name. When the message ends with `but does have X`, the type has a field whose name differs only in case, and Go treats them as different names.

## Why Go does this

In a composite literal with keys, `T{Name: value}`, each key must be a field of the struct. Field names are case-sensitive, and only the fields that start with an upper-case letter can be set from another package. An embedded struct is 1 field, named after its type: `User{Base: Base{ID: 1}}`.

## How to find it

Go to {{file}}:{{line}} and compare the key with the declaration of `{{type}}`:

- Case: `timeout` and `Timeout` are 2 names. For a type from another package, the field you can use starts with an upper-case letter.
- A library type: list its fields with `go doc`, for example `go doc net/http.Client`.
- Your own type: did you rename the field, or does the field live in another struct?
- A value that is computed, not stored: maybe the type has a method or a constructor function for it, instead of a field.

## Example

```go
func NewClient() *http.Client {
	return &http.Client{timeout: 5 * time.Second} // unknown field timeout in struct literal of type http.Client, but does have Timeout
}
```

Use the exported field name:

```go
	return &http.Client{Timeout: 5 * time.Second}
```

## Learn more
