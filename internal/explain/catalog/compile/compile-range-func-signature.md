---
id: compile-range-func-signature
source: compile
match: ^cannot range over (?P<expr>.+?) \((?P<what>.+?)\): func must be func\(yield func\(\.\.\.\) bool\): (?P<reason>.+)$
title: {{expr}} is a function, but not an iterator that range can use
course: 38-logmerge
links:
- The Go spec: For statements with range clause | https://go.dev/ref/spec#For_range
- Package iter | https://pkg.go.dev/iter
- The Go blog: Range Over Function Types | https://go.dev/blog/range-functions
---
## What it means

At {{file}}:{{line}} you range over `{{expr}}`, a function, but its signature is not 1 of the 3 iterator shapes. The end of the message says what is wrong: `{{reason}}`.

## Why Go does this

`range` accepts a function of the form `func(yield func() bool)`, `func(yield func(V) bool)` or `func(yield func(K, V) bool)`, with no result. The `bool` that `yield` returns is the protocol: it is `false` when the loop body stopped early with `break` or `return`. The iterator must then stop calling `yield` and return, which is where it cleans up. Without that result, early exit would not work.

## How to find it

Compare your function type with the 3 shapes, part by part:

- `yield` must return `bool`: `func(yield func(int) bool)`, not `func(yield func(int))`.
- The iterator itself returns nothing.
- Use the named types `iter.Seq[V]` and `iter.Seq2[K, V]` in your signatures. Then the compiler checks the shape where you write the iterator, and the message is clearer.

Inside the iterator, check the result of every call: `if !yield(v) { return }`.

## Example

```go
func Evens(limit int) func(yield func(int)) {
	return func(yield func(int)) {
		for i := 0; i < limit; i += 2 {
			yield(i)
		}
	}
}

for n := range Evens(10) { // cannot range over Evens(10) (...): ... yield func does not return bool
	total += n
}
```

Return an `iter.Seq[int]` and stop when `yield` returns `false`:

```go
func Evens(limit int) iter.Seq[int] {
	return func(yield func(int) bool) {
		for i := 0; i < limit; i += 2 {
			if !yield(i) {
				return
			}
		}
	}
}
```

## Learn more

Exercise 13 practises iterators that clean up when the loop stops early.
