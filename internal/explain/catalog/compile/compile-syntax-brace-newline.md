---
id: compile-syntax-brace-newline
source: compile
match: ^syntax error: unexpected newline, expected \{ after \w+ clause$
match: ^unexpected newline, expecting \{ after \w+ clause(?: \(and \d+ more errors?\))?$
match: ^expected '\{', found newline(?: \(and \d+ more errors?\))?$
match: ^(?:syntax error: )?unexpected semicolon or newline before \{(?: \(and \d+ more errors?\))?$
title: The opening brace must be on the same line
links:
- Effective Go: Semicolons | https://go.dev/doc/effective_go#semicolons
- The Go spec: Semicolons | https://go.dev/ref/spec#Semicolons
- Go FAQ: Why are there braces but no semicolons? | https://go.dev/doc/faq#semicolons
---
## What it means

At {{file}}:{{line}} an opening brace `{` is on the line below the `if`, `for`, `switch` or `func` it belongs to. In Go, the `{` must end the line of the statement or the function signature.

## Why Go does this

Go does not make you type semicolons: the lexer inserts 1 at the end of each line that ends with an identifier, a literal or a closing bracket. After `if n < 0` or `func Answer() int`, that inserted semicolon ends the statement before the `{` arrives. The `{` on the next line then starts something new, which is not valid there. This rule is also why all Go code shares 1 brace style.

## How to find it

Go to {{file}}:{{line}} and look at the end of that line and the start of the next one:

- Move the `{` up to the end of the line with `if`, `for`, `switch`, `select` or `func`.
- A `switch` with its `{` on the next line can even compile with a different meaning: Go reads `switch x;` as a switch on `true`. Errors such as `x is not used` and `cannot convert ... to type bool` point to that.

Format the file to see the standard layout; gofmt cannot format a file with this error, so fix the brace first.

## Example

```go
func Sign(n int) int {
	if n < 0 // unexpected newline, expecting { after if clause
	{
		return -1
	}
	return 1
}
```

Put the brace at the end of the `if` line:

```go
	if n < 0 {
		return -1
	}
```

## Learn more
