package lis2a

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/kardianos/lis2a/e139497"
	"github.com/kardianos/lis2a/lis2a2"
)

func TestRoundTrip(t *testing.T) {
	list := []struct {
		Filename string
		Registry Registry
	}{
		{"lis2a.txt", lis2a2.Registry},
		{"lis2a.txt", e139497.Registry},
		{"esr.txt", e139497.Registry},
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
			t.Logf("GOT: %+v", got)

			encode := NewEncoder(&EncodeOption{TrimTrailingSeparator: true})
			gg, err := encode.Encode(got)
			if err != nil {
				t.Fatal(err)
			}
			gg = bytes.ReplaceAll(gg, []byte{'\r'}, []byte{'\n'})
			t.Logf("Round Trip:\n%s\n", gg)
		})
	}
}
