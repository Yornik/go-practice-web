package fixture

import "log"

func defaultLog(msg string) { log.Print(msg) }

func Log(msg string) {
	if defaultLog != nil {
		defaultLog(msg)
	}
}
