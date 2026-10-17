---
id: staticcheck-sa4006
source: staticcheck
code: SA4006
match: ^this value of (?P<name>\S+) is never used$
title: The value stored in {{name}} is never read
links:
- staticcheck: SA4006 | https://staticcheck.dev/docs/checks/#SA4006
- Effective Go: Errors | https://go.dev/doc/effective_go#errors
- The Go blog: Error handling and Go | https://go.dev/blog/error-handling-and-go
---
## What it means

At this line you store a value in `{{name}}`, but no code reads that value: `{{name}}` gets a new value, or the function returns, first. The work that produced the value is wasted. Often it hides a bug, such as an error that you meant to check.

## Why Go does this

The compiler only rejects a variable that is never read at all. As soon as some line reads `{{name}}`, the compiler is satisfied, even when a single assignment is lost. staticcheck follows each assignment along every path through the function and reports the ones that no path reads.

## How to find it

Go to {{file}}:{{line}}: that assignment is the one whose value is lost. Read down from there to the next assignment to `{{name}}`. What did you mean to do with the first value in between? The common causes:

- An `err` from one call is overwritten by the next call before you check it.
- A starting value, such as `out := []string{}`, that every path replaces anyway.
- A result you computed for a check that you have not written yet.

## Example

```go
port, err := strconv.Atoi(p)
timeout, err := time.ParseDuration(t) // the first err is never checked
if err != nil {
	return err
}
```

Check every error right after the call that returns it:

```go
port, err := strconv.Atoi(p)
if err != nil {
	return fmt.Errorf("port %q: %w", p, err)
}
```

## Learn more
