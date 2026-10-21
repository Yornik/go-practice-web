---
id: compile-pointer-receiver
source: compile
match: ^cannot use (?P<expr>.+?) \((?P<what>.+?)\) as (?P<iface>.+?) value in [^:\n]+: (?P<type>\S+) does not implement .+ \(method (?P<method>\S+) has pointer receiver\)$
title: Only *{{type}} implements {{iface}}, because {{method}} has a pointer receiver
course: 16-redact
links:
- The Go spec: Method sets | https://go.dev/ref/spec#Method_sets
- Go FAQ: Why do T and *T have different method sets? | https://go.dev/doc/faq#different_method_sets
- Go FAQ: Should I define methods on values or pointers? | https://go.dev/doc/faq#methods_on_values_or_pointers
---
## What it means

The method `{{method}}` is declared on the pointer type: `func (x *{{type}}) {{method}}(...)`. So only a `*{{type}}` has that method, and only a `*{{type}}` implements `{{iface}}`. At {{file}}:{{line}} you used `{{expr}}`, a `{{type}}` value, where Go needs an `{{iface}}`.

## Why Go does this

The method set of `T` holds only the methods with receiver `T`; the method set of `*T` holds the methods with receiver `T` and `*T`. On a variable, Go calls a pointer method for you: `v.M()` means `(&v).M()`. But an interface stores its own copy of a value, and nobody can take the address of that copy. A pointer method would change the copy, not your variable, so Go does not let the value implement the interface.

## How to find it

Find where the value is created and decide which form is right:

- Use a pointer: `&{{type}}{...}` or `&v`. This is the usual fix. Error types and types whose methods change state, such as a writer, are almost always used through a pointer.
- Or give `{{method}}` a value receiver, if it does not change the receiver. Then `{{type}}` and `*{{type}}` both implement the interface. Keep the receivers of 1 type consistent.

When the interface is `error`, the caller must also look for the pointer: `errors.As` needs a `*{{type}}` target.

## Example

```go
type NotFoundError struct{ Host string }

func (e *NotFoundError) Error() string { return "not found: " + e.Host }

func Lookup(host string) error {
	return NotFoundError{Host: host} // NotFoundError does not implement error (method Error has pointer receiver)
}
```

Return a pointer, the type that has the method:

```go
	return &NotFoundError{Host: host}
```

## Learn more

The course exercise below practises the method sets of values and of pointers.
