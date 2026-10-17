---
id: vet-httpresponse
source: vet
analyzer: httpresponse
match: ^using (?P<name>\S+) before checking for errors$
title: {{name}} is used before the error of the request is checked
links:
- Package net/http: Client.Do | https://pkg.go.dev/net/http#Client.Do
- Effective Go: Defer | https://go.dev/doc/effective_go#defer
- The httpresponse check of go vet | https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/httpresponse
---
## What it means

You use the response `{{name}}`, often in `defer {{name}}.Body.Close()`, before you check the error that the request returned. When the request fails, for example because the server is down, `{{name}}` is nil. Then `{{name}}.Body` panics with a nil pointer dereference, in the defer or later, and hides the real error.

## Why Go does this

`http.Get` and `Client.Do` return a response and an error. The docs say that on error any response can be ignored. In practice it is nil. Only when the error is nil can you rely on a response with a body that you must close. The compiler cannot know this relation between the 2 results, so `go vet` checks the order.

## How to find it

Go to {{file}}:{{line}} and look at the lines between the call and the `if err != nil`. Every use of `{{name}}` must come after that check, the deferred `Close` included. The same rule holds for every Go function that returns a value and an error: check the error first.

## Example

```go
resp, err := http.Get(url)
defer resp.Body.Close() // panics when err is not nil
if err != nil {
	return 0, err
}
```

Check the error first, then defer the close:

```go
resp, err := http.Get(url)
if err != nil {
	return 0, err
}
defer resp.Body.Close()
```

## Learn more
