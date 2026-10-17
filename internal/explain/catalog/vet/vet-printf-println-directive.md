---
id: vet-printf-println-directive
source: vet
analyzer: printf
match: ^(?P<call>\S+) call has possible Printf formatting directive (?P<verb>%\S+)$
title: {{call}} does not read format verbs such as {{verb}}
links:
- Package fmt: Printing | https://pkg.go.dev/fmt#hdr-Printing
- The printf check of go vet | https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/printf
---
## What it means

`{{call}}` prints its arguments in order, each in its default format. It does not read `{{verb}}` as a verb, so the output contains the characters `{{verb}}` and then the value. `fmt.Println("errors: %d", n)` prints `errors: %d 3`.

## Why Go does this

Go has 2 families of print functions. The functions whose names end in `f`, such as `fmt.Printf`, `log.Printf` and `t.Logf`, read a format string with verbs. The others, such as `Print`, `Println` and `t.Log`, format every argument with `%v` and read no verbs. The compiler accepts both. `go vet` sees a `%` followed by a verb letter in a call that reads no verbs, and suspects that you meant the `f` function. `go test` runs this check before the tests.

## How to find it

Go to {{file}}:{{line}} and decide what you meant:

- A format: switch to the `f` function of the same family. Add `\n` at the end if you need a line break, because `Printf` does not add one.
- A list of values: remove the verb and let the function put spaces between the arguments.

## Example

```go
fmt.Println("errors: %d", n) // prints errors: %d 3
```

Use the function that reads verbs, or remove the verb:

```go
fmt.Printf("errors: %d\n", n)
fmt.Println("errors:", n)
```

## Learn more
