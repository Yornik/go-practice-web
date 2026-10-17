---
id: compile-missing-method
source: compile
match: ^cannot use (?P<expr>.+?) \((?P<what>.+?)\) as (?P<iface>.+?) value in [^:\n]+: (?P<type>.+?) does not implement .+? \((?:missing|unexported) method (?P<method>\S+)\)(?:\n[\s\S]*)?$
title: {{type}} does not implement {{iface}}: the method {{method}} is missing
course: 04-grep
links:
- The Go spec: Interface types | https://go.dev/ref/spec#Interface_types
- Effective Go: Interfaces | https://go.dev/doc/effective_go#interfaces
- Effective Go: Interface checks | https://go.dev/doc/effective_go#blank_implements
---
## What it means

`{{iface}}` is an interface that requires a method `{{method}}`. The type `{{type}}` has no such method, so `{{expr}}` cannot be used as an `{{iface}}` at {{file}}:{{line}}. When the lines `have` and `want` follow the message, `{{type}}` has a method whose name differs only in case, such as `string()` instead of `String()`.

## Why Go does this

A type implements an interface when it has all the methods of the interface, with the same names and signatures. There is no `implements` keyword, so Go checks this where you use a value as the interface. The case of a name matters: a lower-case method is a different, unexported method.

## How to find it

- List what the interface requires: `go doc io.Reader` or `go doc fmt.Stringer`.
- For `error`: a string is not an error. Create 1 with `errors.New("...")` or `fmt.Errorf`, or write a type with an `Error() string` method.
- Did you declare the method on another type, such as a wrapper or an inner field?
- To get this error at the type instead of at each use, add a compile-time check next to the type: `var _ io.Reader = (*Blank)(nil)`.

## Example

```go
func Check(port int) error {
	if port <= 0 {
		return "invalid port" // string does not implement error (missing method Error)
	}
	return nil
}
```

Create an error value:

```go
	if port <= 0 {
		return fmt.Errorf("invalid port %d", port)
	}
```

## Learn more

Exercise 04 practises the io.Reader and io.Writer contracts.
