package fixture

// NotFoundError reports a missing host.
type NotFoundError struct{ Host string }

func (e *NotFoundError) Error() string { return "not found: " + e.Host }

func Lookup(host string) error {
	return NotFoundError{Host: host}
}
