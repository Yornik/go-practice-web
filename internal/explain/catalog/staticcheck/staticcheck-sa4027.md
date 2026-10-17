---
id: staticcheck-sa4027
source: staticcheck
code: SA4027
title: URL.Query returns a copy, so changing it does not change the URL
links:
- staticcheck: SA4027 | https://staticcheck.dev/docs/checks/#SA4027
- Package net/url: URL.Query | https://pkg.go.dev/net/url#URL.Query
- Package net/url: Values.Encode | https://pkg.go.dev/net/url#Values.Encode
---
## What it means

`u.Query()` parses the query string of the URL into a new `url.Values` map. `Set`, `Add` and `Del` on that map change only the map. The URL keeps its old query, and your change is lost at the end of the statement.

## Why Go does this

A `url.URL` stores its query as a string, in the field `RawQuery`. The docs of `URL.Query` say that it parses `RawQuery` and returns the values, so every call builds a new map. Nothing writes that map back to the URL unless you do it. The compiler cannot see this, because changing a map is always valid.

## How to find it

Go to {{file}}:{{line}}. Keep the map in a variable, change it, then encode it back into the URL: `u.RawQuery = q.Encode()`. Note that `Encode` sorts the parameters by key. When you build a new URL, you can also build the `url.Values` first and encode it once.

## Example

```go
u.Query().Set("page", page) // changes a copy
```

Change the values, then write them back:

```go
q := u.Query()
q.Set("page", page)
u.RawQuery = q.Encode()
```

## Learn more
