---
id: staticcheck-sa9002
source: staticcheck
code: SA9002
match: ^file mode '(?P<mode>\d+)' evaluates to (?P<octal>0\d+); did you mean '(?P<want>0\d+)'\?$
title: The file mode {{mode}} is a decimal number; you likely meant {{want}}
course: 24-diskbuffer
links:
- staticcheck: SA9002 | https://staticcheck.dev/docs/checks/#SA9002
- The Go spec: Integer literals | https://go.dev/ref/spec#Integer_literals
- Package os: FileMode | https://pkg.go.dev/os#FileMode
---
## What it means

File permissions are written in octal: `0644` means read and write for the owner and read for everyone else. Without the leading 0, `{{mode}}` is a decimal number, which is {{octal}} in octal. Those are different permissions: with 644 the owner can write the file but not read it. Your program then fails when it opens the file again.

## Why Go does this

The Go spec reads an integer literal with a leading `0` or `0o` as octal, and one without a prefix as decimal. A file mode is a plain number, so the compiler accepts any value. staticcheck reports a decimal mode that looks like an octal one.

## How to find it

Go to {{file}}:{{line}} and write the mode with the prefix `0o`, as in `0o644`. The `o` makes it clear to every reader that the number is octal. Check the other places where you create files or directories too: `os.WriteFile`, `os.Mkdir` and `os.MkdirAll` take a mode as well.

## Example

```go
f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 644)
```

Write the mode in octal:

```go
f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
```

## Learn more

The course exercise below practises creating and appending to files.
