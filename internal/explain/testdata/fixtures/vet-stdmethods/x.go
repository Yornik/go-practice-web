package fixture

import "fmt"

type ParseError struct {
	Line int
	Err  error
}

func (e *ParseError) Error() string { return fmt.Sprintf("line %d: %v", e.Line, e.Err) }

func (e *ParseError) Unwrap() string { return e.Err.Error() }
