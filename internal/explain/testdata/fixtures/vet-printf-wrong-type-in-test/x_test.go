package fixture

import "testing"

func TestReport(t *testing.T) {
	if got := Report("api", 3); got != "3 requests from api" {
		t.Errorf("Report() = %q", got)
	}
}
