package git

import "strings"

func PythonBytesLiteral(s string) string {
	var b strings.Builder
	b.WriteString("b'")
	for _, c := range []byte(s) {
		switch c {
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\\', '\'':
			b.WriteByte('\\')
			b.WriteByte(c)
		default:
			if c >= 0x20 && c <= 0x7e {
				b.WriteByte(c)
			} else {
				b.WriteString(`\x`)
				const hex = "0123456789abcdef"
				b.WriteByte(hex[c>>4])
				b.WriteByte(hex[c&0x0f])
			}
		}
	}
	b.WriteByte('\'')
	return b.String()
}
