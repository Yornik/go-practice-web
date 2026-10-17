package fixture

type Queue struct{ items []string }

func (self *Queue) Len() int { return len(self.items) }
