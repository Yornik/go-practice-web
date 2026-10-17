---
id: compile-syntax-invalid-character
source: compile
match: ^(?:invalid|illegal) character U\+[0-9A-F]+ '(?P<char>.+?)'(?: in identifier)?(?: \(and \d+ more errors?\))?$
match: ^curly quotation mark '(?P<char>.+?)' \(use neutral '"'\)(?: \(and \d+ more errors?\))?$
title: The character {{char}} has no meaning in Go code
links:
- The Go spec: Operators and punctuation | https://go.dev/ref/spec#Operators_and_punctuation
- The Go spec: String literals | https://go.dev/ref/spec#String_literals
- Go FAQ: Why does Go not have the ?: operator? | https://go.dev/doc/faq#Does_Go_have_a_ternary_form
---
## What it means

At {{file}}:{{line}} the code contains the character `{{char}}` outside a string or a comment, and Go has no token that uses it. Common causes: `?` from the `cond ? a : b` operator of other languages, `$` or `#` from shell or scripting languages, and curly quotes such as `“` that a word processor or a web page put in place of `"`.

## Why Go does this

Go has a fixed set of operators and punctuation, and `?`, `$`, `#`, `@` and curly quotes are not part of it. Go has no conditional operator on purpose: the Go FAQ says it led to expressions that are hard to read, and an `if` is clearer. String literals use only the straight double quote `"` or the backquote.

## How to find it

Go to the column of {{file}}:{{line}} and look at the character itself:

- `? :`: write an `if` statement that assigns or returns.
- Curly quotes `“ ”` or `‘ ’`: retype them as `"` or `'`. Code copied from a document or a chat often contains them.
- `#` for a comment: Go comments start with `//` or are enclosed in `/* */`.

## Example

```go
func Max(a, b int) int {
	return a > b ? a : b // invalid character U+003F '?'
}
```

Write the choice as an `if`:

```go
func Max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
```

## Learn more
