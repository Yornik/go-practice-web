---
id: vet-structtag-unexported
source: vet
analyzer: structtag
match: ^struct field (?P<field>\S+) has (?P<key>\S+) tag but is not exported$
title: The field {{field}} has a {{key}} tag, but package {{key}} cannot see it
course: 18-tableprint
links:
- The Go spec: Exported identifiers | https://go.dev/ref/spec#Exported_identifiers
- Package encoding/json: Marshal | https://pkg.go.dev/encoding/json#Marshal
- The structtag check of go vet | https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/structtag
---
## What it means

`{{field}}` starts with a lower-case letter, so it is not exported. The package that reads the `{{key}}` tag encodes and decodes only exported fields. It skips `{{field}}` without an error: the field is missing from the output, and decoding never fills it. The tag has no effect at all.

## Why Go does this

A name is exported when its first character is an upper-case letter, and only exported fields are visible outside their package. encoding/json and encoding/xml live in other packages and use reflection, which follows the same rule: they can read and set only exported fields. The docs of `json.Marshal` say that each exported field becomes a member of the object. `go vet` reports the tag, because it shows that you expected the field in the data.

## How to find it

Go to {{file}}:{{line}} and decide whether the field belongs in the encoded data:

- Yes: export it with an upper-case first letter. The tag still sets the key, so the JSON can keep a lower-case name.
- No: remove the tag, so that no reader expects the field in the output.

A test that compares the JSON shows the missing key. Compare the expected output in the test with what you produce.

## Example

```go
type Build struct {
	Tag    string `json:"tag"`
	commit string `json:"commit"` // never encoded
}
```

Export the field and keep the key in the tag:

```go
	Commit string `json:"commit"`
```

## Learn more

The course exercise below reads struct tags and leaves unexported fields out.
