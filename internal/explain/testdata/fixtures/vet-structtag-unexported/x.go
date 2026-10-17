package fixture

type Health struct {
	Status string   `json:"status"`
	checks []string `json:"checks"`
}

func New(checks []string) Health { return Health{Status: "ok", checks: checks} }
