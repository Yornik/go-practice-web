package fixture

import "io"

type StringWriter interface {
	Write(s string) error
}

func WriteText(w io.Writer, s string) error {
	if sw, ok := w.(StringWriter); ok {
		return sw.Write(s)
	}
	_, err := io.WriteString(w, s)
	return err
}
