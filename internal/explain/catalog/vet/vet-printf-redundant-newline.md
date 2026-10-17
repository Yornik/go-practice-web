---
id: vet-printf-redundant-newline
source: vet
analyzer: printf
match: ^(?P<call>\S+) arg list ends with redundant newline$
title: {{call}} already ends the line, so the extra \n adds an empty line
links:
- Package fmt: Println | https://pkg.go.dev/fmt#Println
- The printf check of go vet | https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/printf
---
## What it means

`{{call}}` writes a newline after its last argument. Your last argument already ends with `\n`, so the output gets 2 line breaks: the line is followed by an empty line. When a test compares the output line by line, it finds a line that it did not expect.

## Why Go does this

The functions whose names end in `ln`, such as `fmt.Println`, `fmt.Fprintln` and `log.Println`, always put a space between the arguments and a newline at the end. An argument that ends with `\n` doubles that newline. `go vet` reports it, because an empty line is rarely what you meant.

## How to find it

Go to {{file}}:{{line}} and look at the last argument of the call. It is a string that ends with `\n`. Choose 1 way to end the line:

- Keep the `ln` function and remove the `\n`.
- Keep the `\n` and switch to a function that adds nothing, such as `fmt.Fprint`, or `fmt.Fprintf` when you also need verbs.

When you write output for a test, compare your lines with the expected output in the test file: every line break counts.

## Example

```go
fmt.Fprintln(w, "status code count\n") // 2 line breaks
```

Let the function end the line:

```go
fmt.Fprintln(w, "status code count")
```

## Learn more
