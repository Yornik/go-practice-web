---
id: compile-duplicate-case
source: compile
match: (?s)^duplicate case (?P<value>.+?)(?: \([^)\n]*\))? in (?P<kind>expression switch|type switch)(?:\n.*)?$
match: ^duplicate key (?P<value>.+) in (?P<kind>map literal)$
title: {{value}} appears twice in the same {{kind}}
links:
- The Go spec: Switch statements | https://go.dev/ref/spec#Switch_statements
- The Go spec: Composite literals | https://go.dev/ref/spec#Composite_literals
- Effective Go: Switch | https://go.dev/doc/effective_go#switch
---
## What it means

The value `{{value}}` appears a second time in the same {{kind}} at {{file}}:{{line}}. In a `switch`, the line `previous case` under the message points to the first one. The second one could never be chosen, or in a map literal it would overwrite the first.

## Why Go does this

A `switch` runs the first case that matches, so a second case with the same constant is dead code, and the compiler reports it. A map holds 1 value per key, so a literal that lists a key twice would silently lose 1 of the values. Both are almost always a copy-and-paste slip or a misunderstanding of what the cases cover.

## How to find it

Find the first occurrence, with the `previous case` line or by searching for `{{value}}`, then decide:

- The 2 cases should do the same thing: keep 1, and list several values in it, `case "warn", "warning":`.
- They should do different things: 1 of them has the wrong value. Check the spelling and the data you expect.
- In a map literal, keep the entry with the right value.

## Example

```go
switch level {
case "warn", "warning":
	return 2
case "error":
	return 3
case "warn": // duplicate case "warn" (constant of type string) in expression switch
	return 1
}
```

Keep each value in exactly 1 case:

```go
switch level {
case "warn", "warning":
	return 2
case "error":
	return 3
}
```

## Learn more
