package fixture

import "encoding/json"

type Config struct {
	Port int `json:"port"`
}

func Decode(b []byte) (Config, error) {
	var c Config
	err := json.Unmarshal(b, c)
	return c, err
}
