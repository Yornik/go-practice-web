package fixture

import (
	"io"
	"os"
)

func Read(path string) ([]byte, error) {
	f, err := os.Open(path)
	defer f.Close()
	if err != nil {
		return nil, err
	}
	return io.ReadAll(f)
}
