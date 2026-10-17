---
id: compile-syntax-missing-comma
source: compile
match: ^syntax error: unexpected .+? in (?P<where>composite literal|argument list|parameter list); possibly missing comma or [)}]$
match: ^missing ',' (?:before newline )?in (?P<where>composite literal|argument list|parameter list)(?: \(and \d+ more errors?\))?$
title: A comma is missing in the {{where}}
links:
- The Go spec: Semicolons | https://go.dev/ref/spec#Semicolons
- Effective Go: Semicolons | https://go.dev/doc/effective_go#semicolons
- The Go spec: Composite literals | https://go.dev/ref/spec#Composite_literals
---
## What it means

The {{where}} at {{file}}:{{line}} needs a comma between 2 elements, or after the last element when the closing bracket is on the next line. Go found the next element, or the end of the line, where it expected a comma.

## Why Go does this

Go inserts a semicolon at the end of a line that ends with a value, an identifier or a closing bracket. In a list that spans several lines, that semicolon would end the list too early. A trailing comma on each line tells Go that the list continues, so Go requires it before a line break. As a bonus, adding or moving a line never touches the line before it.

## How to find it

Go to {{file}}:{{line}}. The position points at the end of the line where the comma is missing, or at the element that follows it.

- A multi-line literal or call: put a comma after every element, the last one included, when the closing `}` or `)` is on its own line.
- Elements on 1 line: separate them with commas, `{"a", "b"}`.
- If you do not want the trailing comma, put the closing bracket on the same line as the last element.

The editor says `missing ',' before newline`; the compiler says `unexpected newline ... possibly missing comma`. Both mean the same.

## Example

```go
var hosts = []string{
	"db-1",
	"db-2" // missing ',' before newline in composite literal
}
```

End the last line with a comma too:

```go
var hosts = []string{
	"db-1",
	"db-2",
}
```

## Learn more
