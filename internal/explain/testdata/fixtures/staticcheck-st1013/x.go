package fixture

import "net/http"

func Busy(w http.ResponseWriter) {
	http.Error(w, "try again later", 503)
}
