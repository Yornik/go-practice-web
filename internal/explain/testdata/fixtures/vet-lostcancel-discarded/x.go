package fixture

import (
	"context"
	"net/http"
	"time"
)

func Fetch(ctx context.Context, url string) (*http.Response, error) {
	ctx, _ = context.WithTimeout(ctx, 5*time.Second)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return http.DefaultClient.Do(req)
}
