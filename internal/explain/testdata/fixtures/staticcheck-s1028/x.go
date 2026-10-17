package fixture

import (
	"errors"
	"fmt"
)

func Missing(key string) error {
	return errors.New(fmt.Sprintf("missing key %q", key))
}
