---
id: compile-nil-value
source: compile
match: ^cannot use nil as (?P<type>.+) value in (?P<context>.+)$
title: nil is not a value of type {{type}}
course: 17-flightrec
links:
- The Go spec: The zero value | https://go.dev/ref/spec#The_zero_value
- The Go spec: Assignability | https://go.dev/ref/spec#Assignability
- The Go blog: An Introduction To Generics | https://go.dev/blog/intro-generics
---
## What it means

At {{file}}:{{line}} you used `nil` where Go needs a value of type `{{type}}`. `nil` is the zero value only of pointers, slices, maps, channels, functions and interfaces. A struct, a string, a number or a boolean cannot be `nil`. A type parameter such as `T` cannot be `nil` either, because `T` can be any type, including `int`.

## Why Go does this

`nil` is not a general "no value". Every type has its own zero value: `0` for numbers, `""` for strings, `false` for booleans, and a struct whose fields all hold their zero values. Value types that can never be `nil` remove a whole class of null-pointer bugs: a `string` is always a valid string.

## How to find it

Decide what the caller should get on this path:

- A struct: return the zero value `{{type}}{}`. With an error next to it, the caller knows it must not use the value.
- A type parameter: declare `var zero T` and return `zero`. That is the zero value of whatever type `T` is.
- When "no value" must be visible in the result itself, the API needs a pointer or an extra `ok` boolean. Check the signature the exercise asks for.

## Example

```go
func Find(hosts []Host, name string) (Host, error) {
	for _, h := range hosts {
		if h.Name == name {
			return h, nil
		}
	}
	return nil, ErrNotFound // cannot use nil as Host value in return statement
}
```

Return the zero value of the struct:

```go
	return Host{}, ErrNotFound
```

## Learn more

The course exercise below practises the zero value of a type parameter.
