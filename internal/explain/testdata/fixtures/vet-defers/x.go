package fixture

import (
	"log"
	"time"
)

func Handle(work func()) {
	start := time.Now()
	defer log.Printf("took %v", time.Since(start))
	work()
}
