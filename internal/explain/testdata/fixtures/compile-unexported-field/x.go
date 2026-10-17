package fixture

import "time"

func Wall(t time.Time) uint64 {
	return t.wall
}
