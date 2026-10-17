package fixture

import "strings"

// First returns the first line that contains word, searching in a
// goroutine.
func First(lines []string, word string) string {
	found := make(chan string)
	go func() {
		for _, l := range lines {
			if strings.Contains(l, word) {
				found <- l
			}
		}
		close(found)
	}()
	for l := range found {
		return l
	}
	return ""
}
