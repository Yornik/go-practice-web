---
id: staticcheck-st1013
source: staticcheck
code: ST1013
match: ^should use constant (?P<want>http\.\w+) instead of numeric literal (?P<have>\d+)$
title: Write {{want}} instead of the number {{have}}
links:
- staticcheck: ST1013 | https://staticcheck.dev/docs/checks/#ST1013
- Package net/http: Constants | https://pkg.go.dev/net/http#pkg-constants
---
## What it means

You pass the HTTP status code as the number {{have}}. Package net/http defines a named constant for every status code, and `{{want}}` says what the number means. A reader then does not need to know the code by heart, and a typo such as 530 for 503 cannot slip in.

## Why Go does this

The status constants of net/http have the same values as the numbers, so the compiler accepts both. staticcheck suggests the name, because it documents the intent. By default it accepts the well-known codes 200, 400, 404 and 500 as plain numbers.

## How to find it

Go to {{file}}:{{line}} and replace {{have}} with `{{want}}`. Do the same for other status numbers in the file. `http.StatusText(code)` returns the standard text for a code, which is useful in error messages.

## Example

```go
http.Error(w, "try again later", 503)
```

Name the status:

```go
http.Error(w, "try again later", http.StatusServiceUnavailable)
```

## Learn more
