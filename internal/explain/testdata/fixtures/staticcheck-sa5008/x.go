package fixture

type Report struct {
	Status string   `json:"status"`
	Errors []string `json:"errors,omitmpty"`
}
