package fixture

type Stack struct{ items []string }

func (q *Stack) Len() int { return len(q.items) }

func (q Stack) Len() int { return len(q.items) }
