package fixture

import "strconv"

func Sum(a, b string) (int, error) {
	total, err := strconv.Atoi(a)
	if err != nil {
		return 0, err
	}
	if b != "" {
		extra, err := strconv.Atoi(b)
		total += extra
	}
	return total, err
}
