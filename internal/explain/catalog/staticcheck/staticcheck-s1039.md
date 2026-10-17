---
id: staticcheck-s1039
source: staticcheck
code: S1039
match: ^unnecessary use of fmt\.(?P<func>\w+)$
title: fmt.{{func}} has nothing to format here
links:
- staticcheck: S1039 | https://staticcheck.dev/docs/checks/#S1039
- Package fmt: Printing | https://pkg.go.dev/fmt#hdr-Printing
---
## What it means

The call to `fmt.{{func}}` gets 1 string and no values to insert, so it returns that string unchanged. Writing the string directly does the same, costs nothing at run time and reads more clearly.

## Why Go does this

A fmt function is useful when it formats values: verbs in a format string, or several operands. With a single string and nothing to format, the call only copies it. staticcheck reports such calls. Often they are left over from a message that once had a value in it.

## How to find it

Go to {{file}}:{{line}} and replace the call with the string itself. If the message was meant to contain a value, add the verb and the argument instead. Check the old string for a `%%`: in a format string it prints 1 percent sign, so a plain string needs a single `%`.

## Example

```go
return fmt.Sprintf("name count")
```

Use the string itself:

```go
return "name count"
```

## Learn more
