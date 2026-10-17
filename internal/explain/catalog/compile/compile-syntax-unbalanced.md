---
id: compile-syntax-unbalanced
source: compile
match: ^syntax error: unexpected (?P<found>EOF|newline), expected (?:comma, : or )?(?P<want>[)}\]])$
match: ^expected '(?P<want>[)}\]])', found '?(?P<found>EOF|newline)'?(?: \(and \d+ more errors?\))?$
title: A bracket is not closed: Go expected {{want}} before {{found}}
links:
- The Go spec: Blocks | https://go.dev/ref/spec#Blocks
- The Go spec: Semicolons | https://go.dev/ref/spec#Semicolons
- Effective Go: Formatting | https://go.dev/doc/effective_go#formatting
---
## What it means

Go reached {{found}} while a bracket was still open, and it expected the closing `{{want}}` first. `EOF` means the end of the file: a `{` somewhere above has no matching `}`. `newline` means the end of the line: a `(` or `[` on this line is not closed.

## Why Go does this

Brackets must pair up: every `{`, `(` and `[` needs its closing partner, in the right order. At the end of a line Go may insert a semicolon, so an expression with an open `(` or `[` cannot go on to the next line without a closing bracket or a trailing operator or comma.

## How to find it

The position is where Go noticed the problem, which is often far below the real mistake.

- `EOF` and `}`: walk through the file from the top and match each `{` with its `}`. Look at the last function you edited, at an `if` or `for` block that lost its `}`, and at a function literal.
- `newline` and `)` or `]`: count the brackets on the line at {{file}}:{{line}}.
- Formatting the file after the fix shows the structure. Wrong indentation in the result points at the block that is still wrong.

## Example

```go
func Sign(n int) int {
	if n < 0 {
		return -1
	return 1
} // syntax error: unexpected EOF, expected }
```

Close the `if` block before the code that follows it:

```go
func Sign(n int) int {
	if n < 0 {
		return -1
	}
	return 1
}
```

## Learn more
