package fixture

import "errors"

type Host struct{ Name string }

var ErrNotFound = errors.New("not found")

func Find(hosts []Host, name string) (Host, error) {
	for _, h := range hosts {
		if h.Name == name {
			return h, nil
		}
	}
	return nil, ErrNotFound
}
