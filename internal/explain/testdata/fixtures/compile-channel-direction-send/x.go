package fixture

func Produce(out <-chan string, words []string) {
	for _, w := range words {
		out <- w
	}
}
