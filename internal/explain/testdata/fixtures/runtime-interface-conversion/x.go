package fixture

// Port returns the "port" setting.
func Port(settings map[string]any) int {
	return settings["port"].(int)
}
