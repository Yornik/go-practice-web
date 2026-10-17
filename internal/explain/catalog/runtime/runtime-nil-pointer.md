---
id: runtime-nil-pointer
source: runtime
match: ^panic: runtime error: invalid memory address or nil pointer dereference$
title: {{func}} used a nil pointer
links:
- The Go spec: Selectors | https://go.dev/ref/spec#Selectors
- The Go spec: Address operators | https://go.dev/ref/spec#Address_operators
- The Go spec: Calls | https://go.dev/ref/spec#Calls
- Effective Go: Composite literals | https://go.dev/doc/effective_go#composite_literals
---
## What it means

At {{file}}:{{line}}, `{{func}}` went through a pointer that is `nil`. It read or wrote a field through a nil pointer, called a method on a nil interface, or called a function value that is `nil`. There is no memory behind `nil`, so the access failed and Go stopped with this panic.

## Why Go does this

The zero value of pointers, functions, interfaces, maps, slices and channels is `nil`. The spec says that a field access through a nil pointer, a method call on a nil interface and a call of a nil function value cause a run-time panic. A method with a pointer receiver can still start with a nil receiver: the panic comes at the first field it touches.

## How to find it

Open {{file}}:{{line}} and list every expression on that line that can be `nil`: `p.field` with a pointer `p`, a call of a function field, a method call on an interface. Then ask where each one got its value:

- A struct made with `T{}` or `var t T` has nil pointer, map and func fields until something sets them.
- A function that returned `nil, err`: did the caller use the value before it checked `err`?
- The line `[signal SIGSEGV ... addr=0x0]` gives the address that was read. A small address such as `0x10` is a field at that offset inside a struct whose pointer is nil.

Print the suspect with `fmt.Printf("%#v\n", x)` right before the line.

## Example

```go
type Server struct {
	cfg *Config
}

func (s *Server) Port() int {
	return s.cfg.Port // s.cfg is nil: nothing set it
}

s := &Server{}
s.Port()
```

Set the pointer where the value is made, so it can never be nil:

```go
func NewServer(cfg *Config) *Server {
	return &Server{cfg: cfg}
}
```

## Learn more
