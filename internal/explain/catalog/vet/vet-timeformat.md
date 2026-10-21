---
id: vet-timeformat
source: vet
analyzer: timeformat
match: ^(?P<bad>2006-02-01) should be (?P<good>2006-01-02)$
title: The layout {{bad}} means year, day, month; you likely meant {{good}}
course: 25-syslog
links:
- Package time: Constants | https://pkg.go.dev/time#pkg-constants
- Package time: Parse | https://pkg.go.dev/time#Parse
- The timeformat check of go vet | https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/timeformat
---
## What it means

Go writes a time layout as an example of 1 fixed reference time: `Mon Jan 2 15:04:05 MST 2006`. In it, `01` is the month and `02` is the day. So `{{bad}}` means year, day, month. Parsing `2024-03-05` with it gives May 3, and `2024-03-25` fails with `month out of range`.

## Why Go does this

Go has no placeholders such as `YYYY-MM-DD`. Each part of the reference time stands for its field: `2006` the year, `01` the month, `02` the day, `15` the hour, `04` the minute, `05` the second. Swapping `01` and `02` still gives a valid layout, so neither the compiler nor `time.Parse` can tell. `go vet` reports this 1 common swap.

## How to find it

Go to {{file}}:{{line}} and read the layout part by part against the reference time. Better still, use a predefined layout from package time: `time.DateOnly` is `2006-01-02`, and `time.RFC3339` is the format with date, time and zone. Test a date with a day above 12: it shows a swap at once.

## Example

```go
t, err := time.Parse("2006-02-01", s) // year, day, month
```

Use the predefined layout:

```go
t, err := time.Parse(time.DateOnly, s)
```

## Learn more

The course exercise below practises time.Parse with a layout.
