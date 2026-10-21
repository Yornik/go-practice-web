---
id: compile-range-iteration-variables
source: compile
match: ^range over (?P<expr>.+?) \((?P<what>.+)\) permits only one iteration variable$
title: A range over {{expr}} gives 1 value per iteration, not 2
course: 38-logmerge
links:
- The Go spec: For statements with range clause | https://go.dev/ref/spec#For_range
- Package iter | https://pkg.go.dev/iter
- The Go blog: Range Over Function Types | https://go.dev/blog/range-functions
---
## What it means

The loop at {{file}}:{{line}} declares 2 iteration variables, such as `for i, v := range {{expr}}`, but `{{expr}}` ({{what}}) produces only 1 value per iteration. There is no index to go with it.

## Why Go does this

What a range loop produces depends on what it ranges over. A slice, an array or a string gives an index and an element; a map gives a key and a value. A channel gives only the received element, an integer `n` gives only the numbers 0 to n-1, and an `iter.Seq[V]` gives only the values it passes to `yield`. An `iter.Seq2[K, V]` gives 2. The spec allows at most 1 iteration variable for a channel or an integer.

## How to find it

Go to {{file}}:{{line}} and look at the type in the message:

- An `iter.Seq[V]`, a channel or an integer: write 1 variable, `for v := range {{expr}}`. If you need a counter, keep it yourself: `i := 0` before the loop and `i++` at its end.
- If you expected an index and a value, check whether you meant to range over a slice instead.
- If you wrote the iterator yourself and want 2 values, return an `iter.Seq2[K, V]` that calls `yield(k, v)`.

## Example

```go
func Collect(seq iter.Seq[string]) []string {
	var out []string
	for _, s := range seq { // range over seq (variable of func type iter.Seq[string]) permits only one iteration variable
		out = append(out, s)
	}
	return out
}
```

Take the 1 value the iterator gives:

```go
	for s := range seq {
		out = append(out, s)
	}
```

## Learn more

Exercise 13 practises iter.Seq and iter.Seq2.
