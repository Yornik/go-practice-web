package fixture

type Team struct {
	Lead Person
}

type Person struct {
	Team Team
}
