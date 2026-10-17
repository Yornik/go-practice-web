---
id: staticcheck-st1005
source: staticcheck
code: ST1005
match: ^error strings should not (?P<rule>be capitalized|end with punctuation or newlines)$
title: Error strings should not {{rule}}
links:
- staticcheck: ST1005 | https://staticcheck.dev/docs/checks/#ST1005
- Go Code Review Comments: Error Strings | https://go.dev/wiki/CodeReviewComments#error-strings
- The Go blog: Error handling and Go | https://go.dev/blog/error-handling-and-go
---
## What it means

The text of this error starts with a capital letter, or ends with a period or a newline. In Go, errors are usually combined: each caller adds context in front, as in `load config: open app.json: no such file or directory`. A capital letter or a period then lands in the middle of that line and looks wrong.

## Why Go does this

The Go Code Review Comments say that error strings should not be capitalized, unless they start with a proper noun or an acronym, and should not end with punctuation, because they are usually printed after other context. Log messages and text for users are not error strings and follow their own style. The compiler does not check text, so staticcheck does.

## How to find it

Go to {{file}}:{{line}} and rewrite the message: start with a lower-case letter, unless the first word is a name such as `HTTP` or a Go identifier, and drop the final period or newline. When the spec prescribes an exact error message, the spec wins: the tests compare the text.

## Example

```go
return errors.New("Empty input.")
```

Write the message so that it fits after other context:

```go
return errors.New("empty input")
```

## Learn more
