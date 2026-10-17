package fixture

import (
	"context"
	"net/http"
)

func Get(url string) (*http.Request, error) {
	return http.NewRequestWithContext(nil, http.MethodGet, url, nil)
}

var _ context.Context
