---
id: staticcheck-sa1004
source: staticcheck
code: SA1004
match: ^sleeping for (?P<n>\d+) nanoseconds is probably a bug; be explicit if it isn't$
title: time.Sleep({{n}}) sleeps for {{n}} nanoseconds
course: 32-retry
links:
- staticcheck: SA1004 | https://staticcheck.dev/docs/checks/#SA1004
- Package time: Duration | https://pkg.go.dev/time#Duration
- Package time: Constants | https://pkg.go.dev/time#pkg-constants
---
## What it means

`time.Sleep` takes a `time.Duration`, and a Duration counts nanoseconds. The number {{n}} without a unit becomes {{n}} nanoseconds, so the call returns almost at once. In many other languages a sleep takes seconds or milliseconds, which makes this an easy mistake. A retry loop with such a delay hammers the server instead of waiting.

## Why Go does this

`time.Duration` is an `int64` count of nanoseconds. An untyped constant such as {{n}} converts to a Duration without a unit, so the compiler accepts it. Package time defines the units as constants: `time.Millisecond`, `time.Second` and so on. You multiply a number by a unit to get a duration.

## How to find it

Go to {{file}}:{{line}} and decide which unit you meant. Multiply the constant by it: `{{n}} * time.Millisecond`. A count in a variable needs a conversion first, because an `int` times a Duration does not compile: `time.Duration(n) * time.Millisecond`. When you really mean nanoseconds, write `time.Nanosecond` so that the reader sees it. Code that you test often takes the sleep as an injected function, so a test can check the duration instead of waiting.

## Example

```go
time.Sleep(100) // 100 nanoseconds
```

Give the duration a unit:

```go
time.Sleep(100 * time.Millisecond)
```

## Learn more

The course exercise below practises delays with an injected sleep.
