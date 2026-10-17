package fixture

import (
	"os"
	"os/signal"
)

func WaitForInterrupt() {
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt)
	<-c
}
