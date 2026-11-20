package lis2a

import (
	"bytes"
	"testing"
	"unicode/utf8"

	"github.com/kardianos/lis2a/lis2a2"
)

func TestWindows1252Table(t *testing.T) {
	// Spot check against the published code page.
	known := map[byte]rune{0x80: '€', 0x8A: 'Š', 0x91: '‘', 0x93: '“', 0x96: '–', 0x99: '™', 0x9F: 'Ÿ'}
	for b, r := range known {
		if got := windows1252[b-0x80]; got != r {
			t.Errorf("0x%02X: got %U, want %U", b, got, r)
		}
	}
	// Every byte 0x80 to 0xFF round trips, except the undefined bytes which decode to C1 controls.
	for i := 0x80; i <= 0xff; i++ {
		s := decodeText([]byte{byte(i)}, CharsetWindows1252)
		r, _ := utf8.DecodeRuneInString(s)
		out, err := appendText(nil, s, CharsetWindows1252)
		switch i {
		case 0x81, 0x8d, 0x8f, 0x90, 0x9d:
			if r != rune(i) || err == nil {
				t.Errorf("0x%02X: got %U err=%v, want C1 control and encode error", i, r, err)
			}
		default:
			if err != nil || !bytes.Equal(out, []byte{byte(i)}) {
				t.Errorf("0x%02X: decoded %U, encoded %q err=%v", i, r, out, err)
			}
		}
	}
}

func TestDecodeCharset(t *testing.T) {
	list := []struct {
		Name    string
		Charset Charset
		Value   string
		Want    string
		Err     bool
	}{
		{"default ASCII", CharsetDefault, "Muller", "Muller", false},
		{"default Latin-1", CharsetDefault, "M\xfcller", "Müller", false},
		{"default UTF-8", CharsetDefault, "M\xc3\xbcller", "Müller", false},
		{"default Windows-1252", CharsetDefault, "\x93Ko\x9ea\x94 \x80", "“Koža” €", false},
		{"default undefined byte", CharsetDefault, "a\x81b", "a\u0081b", false},
		{"default with escape", CharsetDefault, "M\xfcller&S&\xe9", "Müller^é", false},
		{"Windows-1252 forced on UTF-8 bytes", CharsetWindows1252, "M\xc3\xbcller", "MÃ¼ller", false},
		{"Latin-1 forced", CharsetLatin1, "\x80\xfc", "\u0080ü", false},
		{"UTF-8 forced", CharsetUTF8, "M\xc3\xbcller", "Müller", false},
		{"UTF-8 forced on Latin-1 bytes", CharsetUTF8, "M\xfcller", "", true},
	}
	for _, item := range list {
		t.Run(item.Name, func(t *testing.T) {
			in := testHeader + "P|1||||" + item.Value + "\rL|1|N\r"
			v, err := decodeNoPanic(t, &DecodeOption{Charset: item.Charset}, in)
			if item.Err {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got := v.(lis2a2.LIS2_A2).Patient[0].P.PatientName.Last
			if got != item.Want {
				t.Fatalf("got %q, want %q", got, item.Want)
			}
			if !utf8.ValidString(got) {
				t.Fatalf("got invalid UTF-8 %q", got)
			}
		})
	}
}

func TestEncodeCharset(t *testing.T) {
	list := []struct {
		Name    string
		Charset Charset
		Value   string
		Want    string
		Err     bool
	}{
		{"default ASCII", CharsetDefault, "Muller", "Muller", false},
		{"default Latin-1 range", CharsetDefault, "Müller é", "M\xfcller \xe9", false},
		{"default Windows-1252 range", CharsetDefault, "“Koža” €", "\x93Ko\x9ea\x94 \x80", false},
		{"default not representable", CharsetDefault, "李", "", true},
		{"default C1 control", CharsetDefault, "a\u0085b", "", true},
		{"default invalid UTF-8", CharsetDefault, "M\xfcller", "", true},
		{"default escape", CharsetDefault, "ü^é", "\xfc&S&\xe9", false},
		{"Latin-1", CharsetLatin1, "Müller", "M\xfcller", false},
		{"Latin-1 not representable", CharsetLatin1, "€", "", true},
		{"UTF-8", CharsetUTF8, "Müller 李", "Müller 李", false},
		{"UTF-8 invalid", CharsetUTF8, "M\xfcller", "", true},
	}
	for _, item := range list {
		t.Run(item.Name, func(t *testing.T) {
			out, err := encodeNoPanic(t, &EncodeOption{Charset: item.Charset, TrimTrailingSeparator: true}, testMessage(item.Value))
			if item.Err {
				if err == nil {
					t.Fatalf("expected error, got %q", out)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			want := "\rP|1||||" + item.Want + "\r"
			if !bytes.Contains(out, []byte(want)) {
				t.Fatalf("got %q, want it to contain %q", out, want)
			}
		})
	}
}

func TestCharsetRoundTrip(t *testing.T) {
	in := testHeader + "P|1||||\x93Ko\x9ea\x94^M\xfcller\rC|1|L|caf\xe9 \x80 5\x96 10\rL|1|N\r"
	v, err := decodeNoPanic(t, nil, in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := encodeNoPanic(t, &EncodeOption{TrimTrailingSeparator: true}, v)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != in {
		t.Fatalf("got %q\nwant %q", out, in)
	}
}
