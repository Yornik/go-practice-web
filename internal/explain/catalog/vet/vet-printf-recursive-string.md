---
id: vet-printf-recursive-string
source: vet
analyzer: printf
match: ^(?P<call>\S+) format %\S+ with arg (?P<arg>\S+) causes recursive \(\*?(?:[^()]*\.)?(?P<type>[^().]+)\)\.(?P<method>\w+) method call$
match: ^(?P<call>\S+) arg (?P<arg>\S+) causes recursive call to \(\*?(?:[^()]*\.)?(?P<type>[^().]+)\)\.(?P<method>\w+) method$
title: {{type}}.{{method}} calls itself through {{call}}
course: 26-logfmt
links:
- Package fmt: Printing | https://pkg.go.dev/fmt#hdr-Printing
- Package fmt: Stringer | https://pkg.go.dev/fmt#Stringer
- Effective Go: Printing | https://go.dev/doc/effective_go#printing
---
## What it means

Inside the method `{{method}}` of `{{type}}`, you pass the receiver `{{arg}}` to `{{call}}`. fmt sees that `{{arg}}` has a `{{method}}` method and calls it to format the value. That call formats `{{arg}}` again, and so on without end. The program prints nothing useful. It stops with `fatal error: stack overflow`.

## Why Go does this

fmt checks every value before it prints it with `%v`, `%s` or a Print function. A value with an `Error() string` method is printed with that method, and a value with a `String() string` method (a `fmt.Stringer`) with that one. That is how your own types print well everywhere. Inside the method itself, it means that formatting the receiver calls the method again. The docs of package fmt describe this trap and its fix.

## How to find it

Go to the method at {{file}}:{{line}} and find every place that formats the receiver. Format the underlying value instead: convert the receiver to its underlying type, such as `float64`, `int` or `string`. A converted value has no methods, so fmt prints it plainly. Formatting the fields of a struct receiver, field by field, works as well.

## Example

```go
type Celsius float64

func (c Celsius) String() string {
	return fmt.Sprintf("%v°C", c) // calls String again
}
```

Convert the receiver, so fmt prints a plain float64:

```go
	return fmt.Sprintf("%v°C", float64(c))
```

## Learn more

The course exercise below practises a `String` method on your own type.
