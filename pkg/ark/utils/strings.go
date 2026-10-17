package utils

import (
	"fmt"
	"strconv"
	"unicode/utf8"
)

func EscapeNonPrintable(s string) string {
	var data []byte

	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])

		switch {
		case r == utf8.RuneError:
			for _, c := range s[i : i+size] {
				data = fmt.Appendf(data, "\\x%02x", c)
			}

		case r == '\\':
			data = append(data, `\\`...)

		case r == '\a':
			data = append(data, `\a`...)

		case r == '\b':
			data = append(data, `\b`...)

		case r == '\t':
			data = append(data, `\t`...)

		case r == '\n':
			data = append(data, `\n`...)

		case r == '\v':
			data = append(data, `\v`...)

		case r == '\f':
			data = append(data, `\f`...)

		case r == '\r':
			data = append(data, `\r`...)

		case strconv.IsPrint(r):
			data = append(data, s[i:i+size]...)

		default:
			for _, c := range s[i : i+size] {
				data = fmt.Appendf(data, "\\x%02x", c)
			}
		}

		i += size
	}

	return string(data)
}
