---
id: runtime-interface-conversion
source: runtime
match: ^panic: interface conversion: (?P<detail>.+)$
title: A type assertion failed in {{func}}
links:
- The Go spec: Type assertions | https://go.dev/ref/spec#Type_assertions
- Effective Go: Interface conversions and type assertions | https://go.dev/doc/effective_go#interface_conversions
- Package errors: As | https://pkg.go.dev/errors#As
---
## What it means

At {{file}}:{{line}}, `{{func}}` used a type assertion `x.(T)` with 1 result, and the value in `x` did not have type `T`. The message says what it found: `{{detail}}`.

- `interface {} is string, not int`: `x` holds a string, the assertion wanted an int.
- `... is nil, not int`: `x` holds nothing at all, for example a missing map key.
- `*strings.Reader is not io.Closer: missing method Close`: the value does not implement that interface.

## Why Go does this

The type inside an interface value is known only at run time. The spec says that a type assertion that is false causes a run-time panic. The 2-result form `v, ok := x.(T)` never panics: `ok` tells you whether the assertion held. A type switch handles several types in 1 statement.

## How to find it

Find where `x` got its value: a map of `any`, a decoded JSON value, a struct field, an error. Then decide what your code should do when the type is different, and use the 2-result form or a type switch.

For errors, use `errors.As` instead of `err.(*MyError)`. An error wrapped with `fmt.Errorf("...: %w", err)` has another type, so the assertion fails with `error is *fmt.wrapError`, while `errors.As` looks through the wrapping.

## Example

```go
func Port(settings map[string]any) int {
	return settings["port"].(int) // the value is "8080", a string
}
```

Check the type, and return an error that says what you found. The function now returns `(int, error)`:

```go
	port, ok := settings["port"].(int)
	if !ok {
		return 0, fmt.Errorf("port: want an int, got %T", settings["port"])
	}
	return port, nil
```

## Learn more
