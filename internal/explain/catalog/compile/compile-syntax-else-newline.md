---
id: compile-syntax-else-newline
source: compile
match: ^syntax error: unexpected keyword else(?:, .*)?$
match: ^expected statement, found 'else'(?: \(and \d+ more errors?\))?$
title: else must be on the same line as the closing brace of the if
links:
- The Go spec: If statements | https://go.dev/ref/spec#If_statements
- Effective Go: Semicolons | https://go.dev/doc/effective_go#semicolons
- Effective Go: If | https://go.dev/doc/effective_go#if
---
## What it means

At {{file}}:{{line}} an `else` starts a new line, below the `}` that closes the `if` block. Go reads the `if` statement as finished at that `}`, so the `else` stands alone, and a statement cannot start with `else`.

## Why Go does this

Go inserts a semicolon at the end of a line that ends with `}`. After `}` and a line break, the `if` statement is complete, and `else` cannot continue it. Writing `} else {` on 1 line keeps the `else` part of the same statement. The same rule puts the `{` of a block on the line of its statement.

## How to find it

Go to {{file}}:{{line}} and join the `else` with the `}` above it: `} else {` or `} else if cond {`.

Also consider whether you need the `else`. When the `if` block ends with `return`, `break` or `continue`, Go style leaves out the `else` and writes the rest of the code after the block, without extra indentation.

## Example

```go
	if n < 0 {
		return -1
	}
	else { // syntax error: unexpected keyword else, expected }
		return 1
	}
```

Put `else` after the closing brace, or drop it because the `if` returns:

```go
	if n < 0 {
		return -1
	}
	return 1
```

## Learn more
