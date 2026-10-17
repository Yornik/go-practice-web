---
id: compile-imported-and-not-used
source: compile
match: ^"(?P<path>[^"]+)" imported(?: as (?P<alias>\S+))? and not used$
title: The package {{path}} is imported, but this file does not use it
links:
- The Go spec: Import declarations | https://go.dev/ref/spec#Import_declarations
- Effective Go: Unused imports and variables | https://go.dev/doc/effective_go#blank_unused
- Go FAQ: Unused variables and imports | https://go.dev/doc/faq#unused_variables_and_imports
---
## What it means

The file imports `"{{path}}"`, but no code in this file refers to it. Go does not compile a file with an unused import. Imports belong to 1 file: when another file of the same package uses the package, that does not count for this file.

## Why Go does this

The spec makes it illegal to import a package without referring to it. An unused import costs build time and makes the program look as if it depends on something it does not use. The Go designers made it an error instead of a warning, so that this never piles up.

## How to find it

Go to {{file}}:{{line}}, the import line, and ask:

- Did you remove the code that used it? Delete the import.
- Do you call the package by another name? With an alias, `import x "path"`, the file must write `x.Name`, not the package's own name. The message then says `imported as` and the alias.
- Did you mean a different package? `string.Contains` refers to the type `string`, not to the package `strings`, so `"strings"` stays unused and a second error points at `string.Contains`.
- Do you need it only for its side effects at start-up? Then import it as `_ "path"`.

## Example

```go
import (
	"fmt" // "fmt" imported and not used
	"strings"
)

func Upper(s string) string {
	return strings.ToUpper(s)
}
```

Delete the import that the file does not need:

```go
import "strings"
```

## Learn more
