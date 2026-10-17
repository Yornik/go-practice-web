package fixture

type Base struct{ ID int }

type User struct {
	Base
	Name string
}

func Admin() User {
	return User{ID: 1, Name: "admin"}
}
