package fixture

type NotFoundError struct{ Host string }

func (e *NotFoundError) Error() string { return "not found: " + e.Host }

func Kind(err error) string {
	switch err.(type) {
	case NotFoundError:
		return "not found"
	}
	return "other"
}
