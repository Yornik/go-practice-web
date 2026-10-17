package fixture

type Counter struct{ N int }

func (c *Counter) Inc() { c.N++ }

func Bump(counters map[string]Counter, key string) {
	counters[key].Inc()
}
