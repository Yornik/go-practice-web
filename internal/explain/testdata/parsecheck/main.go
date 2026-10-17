// Command parsecheck prints the syntax errors of Go files the way the
// runner's syntax check reports them (internal/runner/syntax.go): go/parser
// without AllErrors, 1 line "file:line:column: message" per error. The
// fixture test builds it with the runner's Go release, so the messages are
// those of that release's go/parser.
package main

import (
	"errors"
	"fmt"
	"go/parser"
	"go/scanner"
	"go/token"
	"os"
)

func main() {
	for _, name := range os.Args[1:] {
		src, err := os.ReadFile(name)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		fset := token.NewFileSet()
		_, err = parser.ParseFile(fset, name, src, parser.SkipObjectResolution)
		var list scanner.ErrorList
		switch {
		case err == nil:
		case errors.As(err, &list):
			for _, e := range list {
				fmt.Printf("%s:%d:%d: %s\n", name, max(e.Pos.Line, 1), max(e.Pos.Column, 1), e.Msg)
			}
		default:
			fmt.Printf("%s:1:1: %s\n", name, err)
		}
	}
}
