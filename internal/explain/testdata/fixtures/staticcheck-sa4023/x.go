package fixture

type ValidationError struct{ Field string }

func (e *ValidationError) Error() string { return "invalid " + e.Field }

func validate(name string) *ValidationError {
	if name == "" {
		return &ValidationError{Field: "name"}
	}
	return nil
}

func Check(name string) error {
	var err error = validate(name)
	if err == nil {
		return nil
	}
	return err
}
