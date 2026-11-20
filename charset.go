package lis2a

import (
	"fmt"
	"unicode/utf8"
)

// Charset is the character set of a message on the wire.
// Decoded strings are always UTF-8, and strings to encode must be UTF-8.
type Charset byte

const (
	// CharsetDefault decodes a message as UTF-8 if the entire message is valid UTF-8,
	// and as Windows-1252 otherwise. It encodes as Windows-1252.
	CharsetDefault Charset = iota

	// CharsetWindows1252 is Windows-1252, the common superset of Latin-1 that
	// uses bytes 0x80 to 0x9F for printable characters such as € and curly quotes.
	// The five bytes it leaves undefined decode to the matching C1 control character.
	CharsetWindows1252

	// CharsetLatin1 is Latin-1 (ISO 8859-1), the character set named by LIS2-A2.
	// Bytes 0x80 to 0x9F decode to C1 control characters.
	CharsetLatin1

	// CharsetUTF8 is UTF-8. Decoding a message that is not valid UTF-8 is an error.
	CharsetUTF8
)

func (cs Charset) String() string {
	switch cs {
	default:
		return fmt.Sprintf("Charset(%d)", byte(cs))
	case CharsetDefault:
		return "Default"
	case CharsetWindows1252:
		return "Windows-1252"
	case CharsetLatin1:
		return "Latin-1"
	case CharsetUTF8:
		return "UTF-8"
	}
}

// windows1252 maps bytes 0x80 to 0x9F to Unicode.
// Generated from the Python cp1252 codec; undefined bytes map to the C1 control of the same value.
var windows1252 = [32]rune{
	0x20AC, // 0x80 €
	0x0081, // 0x81 undefined
	0x201A, // 0x82 ‚
	0x0192, // 0x83 ƒ
	0x201E, // 0x84 „
	0x2026, // 0x85 …
	0x2020, // 0x86 †
	0x2021, // 0x87 ‡
	0x02C6, // 0x88 ˆ
	0x2030, // 0x89 ‰
	0x0160, // 0x8A Š
	0x2039, // 0x8B ‹
	0x0152, // 0x8C Œ
	0x008D, // 0x8D undefined
	0x017D, // 0x8E Ž
	0x008F, // 0x8F undefined
	0x0090, // 0x90 undefined
	0x2018, // 0x91 ‘
	0x2019, // 0x92 ’
	0x201C, // 0x93 “
	0x201D, // 0x94 ”
	0x2022, // 0x95 •
	0x2013, // 0x96 –
	0x2014, // 0x97 —
	0x02DC, // 0x98 ˜
	0x2122, // 0x99 ™
	0x0161, // 0x9A š
	0x203A, // 0x9B ›
	0x0153, // 0x9C œ
	0x009D, // 0x9D undefined
	0x017E, // 0x9E ž
	0x0178, // 0x9F Ÿ
}

// windows1252Encode maps the printable characters of bytes 0x80 to 0x9F back to their byte.
var windows1252Encode = func() map[rune]byte {
	m := make(map[rune]byte, len(windows1252))
	for i, r := range windows1252 {
		if isControl(r) {
			continue
		}
		m[r] = byte(0x80 + i)
	}
	return m
}()

// isControl reports C0 and C1 control characters, including DEL.
func isControl(r rune) bool {
	return r < ' ' || (r >= 0x7f && r <= 0x9f)
}

// resolve returns the character set to decode data with.
func (cs Charset) resolve(data []byte) Charset {
	if cs != CharsetDefault {
		return cs
	}
	if utf8.Valid(data) {
		return CharsetUTF8
	}
	return CharsetWindows1252
}

// decodeText converts b from the resolved character set cs to a UTF-8 string.
// For CharsetUTF8, b must already be valid UTF-8.
func decodeText(b []byte, cs Charset) string {
	i := 0
	for i < len(b) && b[i] < utf8.RuneSelf {
		i++
	}
	if i == len(b) || cs == CharsetUTF8 {
		return string(b)
	}
	buf := make([]byte, i, len(b)+(len(b)-i)*2)
	copy(buf, b[:i])
	for _, c := range b[i:] {
		switch {
		case c < utf8.RuneSelf:
			buf = append(buf, c)
		case c < 0xa0 && cs == CharsetWindows1252:
			buf = utf8.AppendRune(buf, windows1252[c-0x80])
		default:
			buf = utf8.AppendRune(buf, rune(c))
		}
	}
	return string(buf)
}

// appendText appends the UTF-8 string val to dst, encoded in cs.
// Control characters, invalid UTF-8, and characters cs cannot represent are errors.
// Errors give the byte position but not the value.
func appendText(dst []byte, val string, cs Charset) ([]byte, error) {
	for i, r := range val {
		if r == utf8.RuneError {
			if _, size := utf8.DecodeRuneInString(val[i:]); size == 1 {
				return dst, fmt.Errorf("value contains invalid UTF-8 at byte %d", i)
			}
		}
		if isControl(r) {
			return dst, fmt.Errorf("value contains control character U+%04X at byte %d", r, i)
		}
		switch {
		case r < utf8.RuneSelf:
			dst = append(dst, byte(r))
		case cs == CharsetUTF8:
			dst = utf8.AppendRune(dst, r)
		case r <= 0xff:
			dst = append(dst, byte(r))
		case cs == CharsetLatin1:
			return dst, fmt.Errorf("value contains a character at byte %d that cannot be encoded in %v", i, cs)
		default:
			b, ok := windows1252Encode[r]
			if !ok {
				return dst, fmt.Errorf("value contains a character at byte %d that cannot be encoded in %v", i, CharsetWindows1252)
			}
			dst = append(dst, b)
		}
	}
	return dst, nil
}
