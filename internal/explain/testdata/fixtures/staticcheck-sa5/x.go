package fixture

func Wait(ready *bool) {
	for !*ready {
	}
}
