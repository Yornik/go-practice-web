---
id: compile-pointer-to-interface
source: compile
match: ^(?P<expr>.+?)\.(?P<name>[^.\s]+) undefined \(type (?P<type>\*.+) is pointer to (?:interface, not interface|type parameter, not type parameter)\)$
title: {{type}} is a pointer to an interface or a type parameter, which has no methods
links:
- Go FAQ: When should I use a pointer to an interface? | https://go.dev/doc/faq#pointer_to_interface
- The Go spec: Method sets | https://go.dev/ref/spec#Method_sets
- The Go spec: Interface types | https://go.dev/ref/spec#Interface_types
---
## What it means

`{{expr}}` has the type `{{type}}`: a pointer to an interface, or to a type parameter. Such a pointer has no methods, so `{{name}}` does not exist on it, although the interface itself has it. You almost never need a pointer to an interface.

## Why Go does this

An interface value already refers to the value it holds: it stores the concrete type and the value or a pointer to it. The methods belong to the interface type, and the method set of a pointer to an interface is empty. A pointer to a concrete type can satisfy an interface; a pointer to an interface cannot. A type parameter behaves the same way: its methods come from its constraint, not from a pointer to it.

## How to find it

Find where `{{expr}}` gets the type `{{type}}`: a parameter, a field or a variable declared with a `*` in front of an interface such as `io.Reader`, `error` or a type parameter `T`.

- Remove the `*` from that declaration, and the `&` where the value is passed in.
- To pass a value that changes state, such as a buffer, pass the concrete pointer (`*bytes.Buffer`) as the interface. The interface then holds the pointer.
- If a function must give the caller a different interface value, return it.

## Example

```go
func First(r *io.Reader) (byte, error) {
	buf := make([]byte, 1)
	_, err := r.Read(buf) // r.Read undefined (type *io.Reader is pointer to interface, not interface)
	return buf[0], err
}
```

Take the interface itself. A caller passes an `*os.File` or a `*strings.Reader`:

```go
func First(r io.Reader) (byte, error) {
	buf := make([]byte, 1)
	_, err := r.Read(buf)
	return buf[0], err
}
```

## Learn more
