---
id: compile-syntax-unterminated-string
source: compile
match: ^newline in string$
match: ^(?:raw )?string(?: literal)? not terminated(?: \(and \d+ more errors?\))?$
title: The string literal is not closed on this line
course: 26-logfmt
links:
- The Go spec: String literals | https://go.dev/ref/spec#String_literals
- The Go spec: Rune literals | https://go.dev/ref/spec#Rune_literals
- The Go blog: Strings, bytes, runes and characters in Go | https://go.dev/blog/strings
---
## What it means

The string literal that starts at {{file}}:{{line}} has no closing `"` on the same line. Go reports the line break inside the string, and the lines that follow often give more errors, because Go now reads text as code. For a raw string in backquotes, the file ends before the closing backquote.

## Why Go does this

An interpreted string literal, in double quotes, cannot contain a line break; write `\n` for one. A raw string literal, in backquotes, may span lines and takes every character as it is, without escapes. Inside a double-quoted string, `\"` is a quote character and does not end the string, so a missing or extra backslash also leaves a string open.

## How to find it

Go to {{file}}:{{line}} and find where the string should end:

- A closing `"` is missing: add it.
- The text should span several lines: use backquotes, or `\n` and `+` between the lines.
- A quote inside the text: escape it as `\"`, or use backquotes for the whole string.
- A backslash at the end, as in `"C:\"`: write `\\` for a literal backslash.

## Example

```go
const usage = "usage: tool [flags]
  -v  verbose" // newline in string
```

Use a raw string literal, which may contain line breaks:

```go
const usage = `usage: tool [flags]
  -v  verbose`
```

## Learn more

Exercise 01 parses quoted values with escapes.
