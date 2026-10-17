package fixture

import "sync"

func Update(mu *sync.Mutex, update func()) {
	mu.Lock()
	defer mu.Lock()
	update()
}
