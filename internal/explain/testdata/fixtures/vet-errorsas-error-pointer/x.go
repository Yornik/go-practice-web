package fixture

import "errors"

func Cause(err error) error {
	var target error
	if errors.As(err, &target) {
		return target
	}
	return nil
}
