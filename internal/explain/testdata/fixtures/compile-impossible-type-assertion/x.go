package fixture

type NotFoundError struct{ Host string }

func (e *NotFoundError) Error() string { return "not found: " + e.Host }

func MissingHost(err error) string {
	if nf, ok := err.(NotFoundError); ok {
		return nf.Host
	}
	return ""
}
