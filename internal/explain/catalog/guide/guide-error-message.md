---
id: guide-error-message
source: guide
title: How to read a Go error message
links:
- How to Write Go Code | https://go.dev/doc/code
- Command go: Compile packages and dependencies | https://pkg.go.dev/cmd/go#hdr-Compile_packages_and_dependencies
- Command vet | https://pkg.go.dev/cmd/vet
- staticcheck: all checks | https://staticcheck.dev/docs/checks/
---
## The parts of a message

The Go tools print every problem in the same shape:

```text
./retry.go:12:5: undefined: backof
```

- `./retry.go` is the file, relative to the directory where the tool ran.
- `12` is the line and `5` the column. The column counts bytes from 1, so a tab counts as 1.
- Everything after the column is the message: what the tool found, often with the name it is about. The message can contain colons too, as in `undefined: backof`.

A line such as `# example.com/retry` above the messages names the package that failed. Some type errors add indented lines below the message, such as `have (int)` and `want (string)`. They belong to the message above them.

## The first error matters most

Fix the errors from the top down, and run the tools again after each fix. 1 mistake, such as a missing brace or a wrong type, can cause more errors further down that disappear once the first one is fixed. The compiler stops after 10 errors with `too many errors`, so a long list is not complete anyway.

## Who reports it

- The compiler: the code does not compile. Nothing runs until you fix it.
- `go vet`: the code compiles, but a construct is almost always a bug. `go test` runs some vet checks first and stops when they fail.
- staticcheck: the code compiles, but a value is lost or a call cannot do what you expect. The code at the end, such as `SA4006`, is the name of the check: look it up on staticcheck.dev.

## In the editor

Each problem is a marker at its line and an entry in the Problems list. Read the message first, then look at the exact column it points to, and only then at the code around it.

## Learn more
