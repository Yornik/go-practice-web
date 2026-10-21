---
id: staticcheck-sa9005
source: staticcheck
code: SA9005
match: ^struct type '(?P<type>[^']+)' doesn't have any exported fields, nor custom marshaling$
title: The struct {{type}} has no exported fields, so it encodes as an empty object
course: 40-healthcheck
links:
- staticcheck: SA9005 | https://staticcheck.dev/docs/checks/#SA9005
- The Go spec: Exported identifiers | https://go.dev/ref/spec#Exported_identifiers
- Package encoding/json: Marshal | https://pkg.go.dev/encoding/json#Marshal
---
## What it means

You pass a value of `{{type}}` to an encoder such as `json.Marshal`. The struct has no exported fields and no `MarshalJSON` method, so the encoder sees nothing in it. `json.Marshal` returns `{}`, and `json.Unmarshal` fills nothing. Neither reports an error.

## Why Go does this

A field is exported when its name starts with an upper-case letter. encoding/json and encoding/xml live in other packages and use reflection, which can read and set only exported fields. The docs of `json.Marshal` say that each exported field becomes a member of the object. Unexported fields are skipped without a word.

## How to find it

Go to {{file}}:{{line}} and look at the fields of `{{type}}`. Export each field that belongs in the data, and add a tag when the key must be lower-case, as in `json:"lat"`. The type itself may stay unexported: only its fields must be visible.

## Example

```go
type coords struct {
	lat float64 // invisible to encoding/json
	lon float64
}
```

Export the fields and set the keys with tags:

```go
type coords struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}
```

## Learn more

The course exercise below practises JSON output from a struct.
