---
id: runtime-range-func-continued
source: runtime
match: ^panic: runtime error: range function continued iteration after (?P<when>function for loop body returned false|loop body panic|whole loop exit)$
title: An iterator called yield again after the loop had stopped
course: 13-logmerge
links:
- The Go spec: For statements with range clause | https://go.dev/ref/spec#For_range
- Package iter | https://pkg.go.dev/iter
- The Go blog: Range Over Function Types | https://go.dev/blog/range-functions
---
## What it means

A `for ... range` loop over an iterator function stopped early: the loop body ran `break` or `return`, or panicked. The iterator then called `yield` again anyway. The top frame of the exercise package, `{{func}}` at {{file}}:{{line}}, is usually the loop body; its name ends in `-range` and a number. The frame below it is the iterator function that made the extra call.

## Why Go does this

A range-over-func loop calls your iterator with a `yield` function. Each call of `yield` runs the loop body once and returns `true` to ask for more. The spec says that when the loop body stops, `yield` returns `false` and must not be called again. The loop is gone at that point, so Go panics instead of running code that the caller already left.

## How to find it

Go to the iterator in the frame under the loop body and find every call of `yield`. Each one must check the result and stop at once when it is `false`:

- A bare `yield(v)` statement ignores the result.
- A `break` that only leaves an inner loop of the iterator keeps the outer loop going. Use `return`.
- An iterator that wraps another must stop the inner one too, by returning `false` from its own callback.

After it stops, the iterator should still clean up what it opened or started; a `defer` in the iterator does that.

## Example

```go
func Fields(s string) iter.Seq[string] {
	return func(yield func(string) bool) {
		for _, f := range strings.Fields(s) {
			yield(f) // ignores false: the loop keeps going
		}
	}
}
```

Stop as soon as `yield` returns false:

```go
		for _, f := range strings.Fields(s) {
			if !yield(f) {
				return
			}
		}
```

## Learn more

The course exercise below practises iterators and cleanup when the caller stops early.
