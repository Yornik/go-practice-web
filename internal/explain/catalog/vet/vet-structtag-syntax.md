---
id: vet-structtag-syntax
source: vet
analyzer: structtag
match: ^struct field tag (?P<tag>.+) not compatible with reflect\.StructTag\.Get: (?P<problem>.+)$
title: The struct tag {{tag}} is not in the key:"value" format
course: 06-envconfig
links:
- The Go spec: Struct types | https://go.dev/ref/spec#Struct_types
- Package reflect: StructTag | https://pkg.go.dev/reflect#StructTag
- Package encoding/json: Marshal | https://pkg.go.dev/encoding/json#Marshal
---
## What it means

The tag {{tag}} breaks the format that tags must follow: {{problem}}. Packages such as encoding/json read a tag with `reflect.StructTag.Get`. For a malformed tag, Get finds nothing, or stops at the mistake. The package then ignores your tag without an error: json uses the field name as the key, and options such as `omitempty` are lost.

## Why Go does this

To the compiler a tag is a plain string, so any string compiles. The docs of `reflect.StructTag` define the convention: `key:"value"` pairs, separated by spaces, and every value in double quotes. `go vet` checks every tag against that convention, because the packages that read tags never report a broken one.

## How to find it

Go to {{file}}:{{line}} and compare the tag with the format, 1 character at a time. The problem text tells you where to look:

- `bad syntax for struct tag value`: the value is not in double quotes, as in `json:port`.
- `key:"value" pairs not separated by spaces`: something follows the closing quote. Options belong inside the quotes, after a comma: `json:"port,omitempty"`.
- 2 keys need a space between them: `json:"port" env:"PORT"`.

## Example

```go
type Config struct {
	Port int    `json:port`            // no quotes
	Mode string `json:"mode",omitempty` // option outside the quotes
}
```

Quote every value and keep the options inside the quotes:

```go
type Config struct {
	Port int    `json:"port"`
	Mode string `json:"mode,omitempty"`
}
```

## Learn more

The course exercise below practises reading struct tags with reflect.
