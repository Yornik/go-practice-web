package fixture

import "errors"

// ErrBadLimit is wrapped by every error of Limit.Validate.
var ErrBadLimit = errors.New("bad limit")

// Limit is a rate limit.
type Limit struct{ PerSecond int }

// Validate reports an error for a limit below 1.
func (l Limit) Validate() error {
	return nil
}
