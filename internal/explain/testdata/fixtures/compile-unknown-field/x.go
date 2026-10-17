package fixture

import (
	"net/http"
	"time"
)

func NewClient() *http.Client {
	return &http.Client{timeout: 5 * time.Second}
}
