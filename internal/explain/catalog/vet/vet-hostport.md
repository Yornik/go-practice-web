---
id: vet-hostport
source: vet
analyzer: hostport
match: ^address format (?P<format>"[^"]*") does not work with IPv6(?: \(passed to (?P<call>\S+) at L(?P<at>\d+)\))?$
title: The address format {{format}} breaks for IPv6 hosts
course: 18-certcheck
links:
- Package net: JoinHostPort | https://pkg.go.dev/net#JoinHostPort
- Package net: SplitHostPort | https://pkg.go.dev/net#SplitHostPort
- The hostport check of go vet | https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/hostport
---
## What it means

You build a network address with `fmt.Sprintf` and the format {{format}}. An IPv6 host contains colons itself. The host `::1` and port 443 become `::1:443`, and nobody can tell where the host ends. `net.Dial` fails with `too many colons in address`. It works in your tests with `localhost` and breaks in production on an IPv6 address.

## Why Go does this

An address with an IPv6 literal must put the host in brackets: `[::1]:443`. The docs of `net.JoinHostPort` say that it adds the brackets when the host contains a colon. The compiler cannot know what a format string builds, so `go vet` looks for host:port formats whose result goes to a dial function.

## How to find it

Go to {{file}}:{{line}} and replace the `fmt.Sprintf` with `net.JoinHostPort`. It takes the port as a string, so convert a number with `strconv.Itoa`. The reverse holds as well: split an address with `net.SplitHostPort`, never with `strings.Split` on `:`.

## Example

```go
addr := fmt.Sprintf("%s:%d", host, port) // "::1:443" for IPv6
conn, err := net.Dial("tcp", addr)
```

Let package net join host and port:

```go
addr := net.JoinHostPort(host, strconv.Itoa(port))
conn, err := net.Dial("tcp", addr)
```

## Learn more

The course exercise below practises dialing host:port addresses.
