---
id: runtime-ticker-non-positive
source: runtime
match: ^panic: non-positive interval for (?P<call>NewTicker|Ticker\.Reset)$
title: {{call}} got an interval of 0 or less in {{func}}
links:
- Package time: NewTicker | https://pkg.go.dev/time#NewTicker
- Package time: Ticker.Reset | https://pkg.go.dev/time#Ticker.Reset
- Package time: Duration | https://pkg.go.dev/time#Duration
---
## What it means

At {{file}}:{{line}}, `{{func}}` called `time.{{call}}` with a duration that is 0 or negative. A ticker must tick at some interval, so package time panics.

## Why Go does this

The docs of `time.NewTicker` say that the duration must be greater than zero, and that `NewTicker` panics otherwise. A ticker repeats, and a period of 0 or less has no meaning for it, so the package rejects it at once, where the mistake is. Timers are different: `time.NewTimer(0)` and `time.After(0)` are allowed and fire at once.

## How to find it

Print the duration right before the call, and find where it comes from:

- A field of an options or config struct that the caller left at its zero value. Does your function promise a default for 0? Then apply the default before you create the ticker.
- A computation such as `total / time.Duration(n)` that rounds down to 0 for a large `n`, or a subtraction that went negative.
- A plain number used as a duration: `time.Duration(5)` is 5 nanoseconds, not 5 seconds. Multiply by a unit: `5 * time.Second`.

Validate durations where they enter your code, in a constructor or at the start of the function, so the error names the setting.

## Example

```go
func Poll(o PollOptions, f func()) {
	t := time.NewTicker(o.Interval) // 0 when the caller did not set it
	defer t.Stop()
	// ...
}
```

Apply the documented default first:

```go
	interval := o.Interval
	if interval <= 0 {
		interval = time.Second
	}
	t := time.NewTicker(interval)
```

## Learn more
