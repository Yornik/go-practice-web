package fixture

import "net/http"

func Status(url string) (int, error) {
	resp, err := http.Get(url)
	defer resp.Body.Close()
	if err != nil {
		return 0, err
	}
	return resp.StatusCode, nil
}
