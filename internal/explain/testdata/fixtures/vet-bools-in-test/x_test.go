package fixture

import "testing"

func TestSuccess(t *testing.T) {
	if !Success(200) {
		t.Error("200 is a success")
	}
}
