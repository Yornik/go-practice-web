package fixture

import "encoding/json"

type status struct {
	healthy bool
	checks  []string
}

func Encode(healthy bool, checks []string) ([]byte, error) {
	return json.Marshal(status{healthy: healthy, checks: checks})
}
