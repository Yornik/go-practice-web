---
id: vet-unmarshal
source: vet
analyzer: unmarshal
match: ^call of (?P<func>\S+) passes non-pointer(?: as second argument)?$
title: {{func}} needs a pointer to fill your variable
links:
- Package encoding/json: Unmarshal | https://pkg.go.dev/encoding/json#Unmarshal
- Effective Go: Pointers vs. Values | https://go.dev/doc/effective_go#pointers_vs_values
- The unmarshal check of go vet | https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/unmarshal
---
## What it means

`{{func}}` decodes data into the value that you pass. You passed the value itself, not a pointer to it, so the function gets a copy and cannot change your variable. `json.Unmarshal` returns an error such as `json: Unmarshal(non-pointer fixture.Config)`, and your variable keeps its zero value.

## Why Go does this

Go passes every argument by value. A function can change a variable of its caller only through a pointer. `{{func}}` takes its target as `any`, so the compiler accepts a value too. The docs of `json.Unmarshal` say it returns an error when its target is nil or not a pointer. `go vet` checks the decode calls of encoding/json, encoding/xml, encoding/gob and encoding/asn1.

## How to find it

Go to {{file}}:{{line}} and add `&` before the target argument. Then look at the error that the call returns: check it, because it already told you about this mistake. A decoder works the same way: `dec.Decode(&v)`.

## Example

```go
var c Config
err := json.Unmarshal(b, c) // c is a copy; it stays empty
```

Pass the address of the variable:

```go
var c Config
err := json.Unmarshal(b, &c)
```

## Learn more
