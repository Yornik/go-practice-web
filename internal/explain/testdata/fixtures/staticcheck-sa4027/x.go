package fixture

import "net/url"

func WithPage(u *url.URL, page string) *url.URL {
	u.Query().Set("page", page)
	return u
}
