package lis2a

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/kardianos/lis2a/lis2a2"
)

func TestRoundTrip(t *testing.T) {
	list := []struct {
		Filename string
		Registry Registry
	}{
		{"lis2a.txt", lis2a2.Registry},
		{"esr.txt", lis2a2.Registry},
	}
	for _, item := range list {
		t.Run(item.Filename, func(t *testing.T) {
			fn := filepath.Join("testdata", item.Filename)
			ff, err := os.ReadFile(fn)
			if err != nil {
				t.Fatal(err)
			}

			decode := NewDecoder(item.Registry, nil)
			got, err := decode.Decode(ff)
			if err != nil {
				t.Fatal(err)
			}

			encode := NewEncoder(&EncodeOption{TrimTrailingSeparator: true})
			gg, err := encode.Encode(got)
			if err != nil {
				t.Fatal(err)
			}
			gg = bytes.ReplaceAll(gg, []byte{'\r'}, []byte{'\n'})
			t.Logf("Round Trip:\n%s\n", gg)
			ffTrim := trimTrailing(ff)

			if err := lineEqual(ffTrim, gg); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// Remove trailing hat "^" within fields.
var matchTrailingHat = regexp.MustCompile(`(\^+)(\||$)`)

func trimTrailing(bb []byte) []byte {
	lines := split(bb)

	for i, line := range lines {
		x := matchTrailingHat.ReplaceAll(line, []byte("$2"))
		x = bytes.TrimRight(x, "|")
		lines[i] = x
	}
	return bytes.Join(lines, []byte{'\n'})
}

func split(bb []byte) [][]byte {
	return bytes.FieldsFunc(bb, func(r rune) bool {
		switch r {
		default:
			return false
		case '\r', '\n':
			return true
		}
	})
}

func lineEqual(aa, bb []byte) error {
	al, bl := split(aa), split(bb)
	ai, bi := len(al), len(bl)
	ct := ai
	if bi < ct {
		ct = bi
	}
	if ct == 0 {
		if ai != bi {
			return fmt.Errorf("different line lengths: %d, %d", ai, bi)
		}
		return nil
	}

	for i := 0; i < ct; i++ {
		a, b := al[i], bl[i]
		if !bytes.Equal(a, b) {
			return fmt.Errorf("line %d %q vs %q", i+1, a, b)
		}
	}
	if ai != bi {
		return fmt.Errorf("different line lengths: %d, %d", ai, bi)
	}
	return nil
}
