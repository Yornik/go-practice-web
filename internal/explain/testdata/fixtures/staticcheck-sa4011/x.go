package fixture

func Drain(events <-chan string, done <-chan struct{}, handle func(string)) {
	for {
		select {
		case e := <-events:
			handle(e)
		case <-done:
			break
		}
	}
}
