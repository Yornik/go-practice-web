package fixture

import "bytes"

func Render(lines []string) string {
	var buf bytes.Buffer
	for _, l := range lines {
		buf.WriteString(l)
		buf.WriteByte('\n')
	}
	return string(buf.Bytes())
}
