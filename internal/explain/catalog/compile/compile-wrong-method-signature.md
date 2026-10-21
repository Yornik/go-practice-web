---
id: compile-wrong-method-signature
source: compile
match: ^cannot use (?P<expr>.+?) \((?P<what>.+?)\) as (?P<iface>.+?) value in [^:\n]+: (?P<type>.+?) does not implement .+? \(wrong type for method (?P<method>\S+)\)(?:\n\s*have (?P<have>.+)\n\s*want (?P<want>.+))?$
title: {{type}} has a method {{method}}, but not with the signature {{iface}} needs
course: 16-redact
links:
- The Go spec: Implementing an interface | https://go.dev/ref/spec#Implementing_an_interface
- The Go spec: Interface types | https://go.dev/ref/spec#Interface_types
- Package io: Writer | https://pkg.go.dev/io#Writer
---
## What it means

`{{type}}` has a method `{{method}}`, but its parameters or results differ from what `{{iface}}` declares, so `{{expr}}` is not an `{{iface}}`. The 2 lines under the message compare them: `have {{have}}` is your method, `want {{want}}` is the method the interface requires.

## Why Go does this

To implement an interface, a method must have the same name and an identical signature: the same parameter types and the same result types, in the same order. There is no "close enough". The signature is part of the contract: an `io.Writer` must return an error, because a caller needs to know when a write fails.

## How to find it

Compare `have` and `want` word by word:

- The number of results, for example a missing `error`.
- Pointer or value: `*http.Request` is not `http.Request`.
- The order and the types of the parameters, such as `[]byte` and `string`.

Copy the exact signature from the documentation, for example `go doc io.Writer`. After you change the signature, update every `return` in the method.

## Example

```go
type Tally struct{ N int }

func (t *Tally) Write(p []byte) int {
	t.N += len(p)
	return len(p)
}

var _ io.Writer = &Tally{}
```

```text
*Tally does not implement io.Writer (wrong type for method Write)
		have Write([]byte) int
		want Write([]byte) (int, error)
```

Use the signature of the interface:

```go
func (t *Tally) Write(p []byte) (int, error) {
	t.N += len(p)
	return len(p), nil
}
```

## Learn more

The course exercise below adds the methods that `fmt`, `encoding/json` and `log/slog` look for, and each signature must match exactly.
