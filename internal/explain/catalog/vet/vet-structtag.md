---
id: vet-structtag
source: vet
analyzer: structtag
title: A struct tag breaks a rule of the package that reads it
course: 31-envconfig
links:
- Package reflect: StructTag | https://pkg.go.dev/reflect#StructTag
- Package encoding/json: Marshal | https://pkg.go.dev/encoding/json#Marshal
- The structtag check of go vet | https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/structtag
---
## What it means

`go vet` found a struct tag at {{file}}:{{line}} that its package cannot use as written. The message names the field and the problem. A common case: 2 fields of the same struct claim the same JSON or XML name. encoding/json then encodes neither of them, and no error tells you.

## Why Go does this

To the compiler a tag is a plain string. The packages that read tags, such as encoding/json, decide at run time what a tag means, and they ignore what they cannot use. The docs of `json.Marshal` describe the rule for 2 fields with the same name at the same level: when neither one wins, both are left out. `go vet` checks the tag format and, for json and xml, duplicate names and unexported fields.

## How to find it

Read the message at {{file}}:{{line}}. It names the field, the tag key and, for a duplicate, the position of the other field. Compare the 2 fields and give each its own name. Then check the output in a test: every field you expect must be there.

## Example

```go
type Event struct {
	ID   string `json:"id"`
	Kind string `json:"id"` // the same name: json encodes neither field
}
```

Give each field its own key:

```go
	Kind string `json:"kind"`
```

## Learn more
