package fixture

import "fmt"

func ExampleTitle() {
	fmt.Println(Title("hello world"))
	fmt.Println(Title("go"))
	// Output:
	// Hello World
	// Go
}
