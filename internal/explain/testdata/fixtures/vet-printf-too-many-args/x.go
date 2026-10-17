package fixture

import (
	"log"
	"time"
)

func Retry(attempt int, delay time.Duration, err error) {
	log.Printf("retry %d after %v", attempt, delay, err)
}
