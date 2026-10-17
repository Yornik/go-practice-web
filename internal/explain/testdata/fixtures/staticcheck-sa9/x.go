package fixture

import "os"

func Process(paths <-chan string) {
	for p := range paths {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		defer f.Close()
	}
}
