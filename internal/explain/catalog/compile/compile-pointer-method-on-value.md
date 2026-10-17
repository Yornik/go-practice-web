---
id: compile-pointer-method-on-value
source: compile
match: ^cannot call pointer method (?P<method>\S+) on (?P<type>.+)$
title: {{method}} needs the address of a {{type}}, and this value has none
course: 02-accesslog
links:
- The Go spec: Calls | https://go.dev/ref/spec#Calls
- The Go spec: Address operators | https://go.dev/ref/spec#Address_operators
- The Go blog: Go maps in action | https://go.dev/blog/maps
---
## What it means

The method `{{method}}` has a pointer receiver, `*{{type}}`, so Go must take the address of the value to call it. At {{file}}:{{line}} the value is not addressable: usually a map element, such as `m[k].{{method}}()`, or the result of a function call.

## Why Go does this

For a variable `v`, Go rewrites `v.M()` to `(&v).M()`. That only works when the value has an address. A map element has none that you may keep: the map moves its elements when it grows, so a pointer into it would soon point to the wrong place. A function result is a temporary value; a method that changes it would change something that nobody can see afterwards.

## How to find it

Go to {{file}}:{{line}} and decide where the changed value should live:

- In a map: store pointers, `map[string]*{{type}}`. Then `m[k].{{method}}()` changes the value that the map points to.
- Or copy the element out, call the method, and store the copy back: `v := m[k]`, `v.{{method}}()`, `m[k] = v`.
- A function result: assign it to a variable first, then call the method on the variable.

## Example

```go
func Bump(counters map[string]Counter, key string) {
	counters[key].Inc() // cannot call pointer method Inc on Counter
}
```

Copy the element out, change it, and store it back:

```go
func Bump(counters map[string]Counter, key string) {
	c := counters[key]
	c.Inc()
	counters[key] = c
}
```

## Learn more

Exercise 02 practises maps of structs and of pointers.
