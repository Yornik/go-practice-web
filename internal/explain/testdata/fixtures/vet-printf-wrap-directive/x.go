package fixture

import (
	"log"
	"os"
)

func Load(path string) []byte {
	b, err := os.ReadFile(path)
	if err != nil {
		log.Printf("load %s: %w", path, err)
	}
	return b
}
