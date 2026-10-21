---
id: runtime-close-nil-channel
source: runtime
match: ^panic: close of nil channel$
title: {{func}} closed a channel that was never made
course: 46-broadcaster
links:
- The Go spec: Close | https://go.dev/ref/spec#Close
- The Go spec: Channel types | https://go.dev/ref/spec#Channel_types
- The Go spec: Making slices, maps and channels | https://go.dev/ref/spec#Making_slices_maps_and_channels
---
## What it means

At {{file}}:{{line}}, `{{func}}` called `close` on a channel variable that holds `nil`. The variable or field exists, but no channel was ever created with `make`.

## Why Go does this

The zero value of a channel type is `nil`, and the spec says that closing the nil channel causes a run-time panic. A nil channel is not broken in other ways: a send or a receive on it blocks forever. So a missing `make` often shows up first as a test that hangs, and only the `close` panics.

A nil channel has a real use: in a `select`, a case on a nil channel is never ready, which turns that case off.

## How to find it

Find where the channel should be created:

- A struct field: a value made with `T{}` or `var t T` leaves the channel nil. Does a constructor make it, and does the caller use that constructor?
- A field that a `Start` or `Run` method creates: is the close in `Stop` reached before `Start` ran?
- A local `var ch chan T` without `make`.

Create the channel together with the value that owns it, in the constructor, so it is never nil when other methods run.

## Example

```go
type Poller struct {
	done chan struct{}
}

func (p *Poller) Stop() {
	close(p.done) // done is nil in a Poller{}
}
```

Make the channel in the constructor:

```go
func NewPoller() *Poller {
	return &Poller{done: make(chan struct{})}
}
```

## Learn more
