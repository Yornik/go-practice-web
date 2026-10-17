---
id: staticcheck-sa5008
source: staticcheck
code: SA5008
title: A struct tag has an option or format that its encoder does not know
course: 15-healthcheck
links:
- staticcheck: SA5008 | https://staticcheck.dev/docs/checks/#SA5008
- Package encoding/json: Marshal | https://pkg.go.dev/encoding/json#Marshal
- Package reflect: StructTag | https://pkg.go.dev/reflect#StructTag
---
## What it means

The tag of the field at {{file}}:{{line}} has a mistake that the encoder ignores without an error. An option name with a typo, such as `omitmpty`, is the common case. The field is then encoded as if the option was not there: an empty list shows up in the JSON, although you asked to leave it out.

## Why Go does this

To the compiler a tag is a plain string. encoding/json reads the options that it knows and skips everything else. The docs of `json.Marshal` list them: `omitempty`, `omitzero` and `string`, and the name `-` to leave a field out. staticcheck checks the tags for encoding/json and encoding/xml against those rules.

## How to find it

Read the message: it names the unknown option or the broken part of the tag. Compare the tag with the examples in the docs of `json.Marshal`. The format is `json:"name,option"`: the name first, then the options, each after a comma, all inside 1 pair of quotes. A test that compares the JSON output shows whether the option works.

## Example

```go
Errors []string `json:"errors,omitmpty"` // unknown option: ignored
```

Spell the option the way the docs do:

```go
Errors []string `json:"errors,omitempty"`
```

## Learn more

The course exercise below practises JSON output with struct tags.
