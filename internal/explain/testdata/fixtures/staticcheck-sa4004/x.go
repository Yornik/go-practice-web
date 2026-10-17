package fixture

import "errors"

type Item struct{ ID string }

var ErrNotFound = errors.New("not found")

func Find(items []Item, id string) (Item, error) {
	for _, item := range items {
		if item.ID == id {
			return item, nil
		}
		return Item{}, ErrNotFound
	}
	return Item{}, ErrNotFound
}
