package lis2a

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kardianos/lis2a/lis2a2"
)

const testHeader = "H|\\^&||||||||||P|NCCLS LIS2-A|20221004013849\r"

// decodeNoPanic decodes in and fails the test on a panic.
func decodeNoPanic(t *testing.T, opt *DecodeOption, in string) (v any, err error) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic: %v", r)
		}
	}()
	return NewDecoder(lis2a2.Registry, opt).Decode([]byte(in))
}

func TestDecodeMalformed(t *testing.T) {
	list := []struct {
		Name  string
		Input string
	}{
		{"duplicate H", testHeader + "L|1|N\r" + testHeader + "L|1|N\r"},
		{"duplicate L", testHeader + "P|1\rL|1|N\rL|1|N\r"},
		{"duplicate M", testHeader + "M|1\rM|2\rL|1|N\r"},
		{"ISO date", testHeader + "P|1||||||2020-01-01\rL|1|N\r"},
		{"date with early zone sign", testHeader + "P|1||||||1-34567890123456789\rL|1|N\r"},
		{"header date digit and plus", "H|000||||||||||||1+"},
		{"segment after L", testHeader + "P|1\rL|1|N\rP|2\r"},
		{"R without O", testHeader + "P|1\rR|1|^^^GLU|99\rL|1|N\r"},
		{"C before P", testHeader + "C|1|L|note\rP|1\rL|1|N\r"},
		{"M after P", testHeader + "P|1\rM|1\rL|1|N\r"},
		{"repeated delimiters", "H||||||||||||||P|NCCLS LIS2-A\rL|1|N\r"},
		{"alphanumeric delimiter", "H|a^&|||||||||||P|NCCLS LIS2-A\rL|1|N\r"},
		{"short header", "H|\\^"},
	}
	for _, item := range list {
		t.Run(item.Name, func(t *testing.T) {
			_, err := decodeNoPanic(t, nil, item.Input)
			if err == nil {
				t.Fatal("expected error")
			}
			t.Log(err)
		})
	}
}

func TestDecodeGroupAttribution(t *testing.T) {
	in := testHeader +
		"P|1||||ALICE\r" +
		"O|1|SPEC-ALICE\r" +
		"R|1|^^^GLU|5\r" +
		"P|2||||BOB\r" +
		"R|1|^^^GLU|700\r" + // No O record for BOB.
		"L|1|N\r"

	_, err := decodeNoPanic(t, nil, in)
	if err == nil {
		t.Fatal("strict: expected error for R without O")
	}

	v, err := decodeNoPanic(t, &DecodeOption{LenientGroup: true}, in)
	if err != nil {
		t.Fatal(err)
	}
	type row struct{ patient, specimen, value string }
	var got []row
	for _, pg := range v.(lis2a2.LIS2_A2).Patient {
		for _, og := range pg.Order {
			for _, rg := range og.Result {
				got = append(got, row{pg.P.PatientName.Last, og.Order.ID.ID, rg.Result.Value.Value})
			}
		}
	}
	want := []row{
		{"ALICE", "SPEC-ALICE", "5"},
		{"BOB", "", "700"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestDecodeEmptyRecordNoPhantomGroup(t *testing.T) {
	in := testHeader + "P|1\rO|1|S1\rO\rR|1|^^^GLU|5\rL|1|N\r"
	v, err := decodeNoPanic(t, nil, in)
	if err != nil {
		t.Fatal(err)
	}
	orders := v.(lis2a2.LIS2_A2).Patient[0].Order
	if len(orders) != 2 {
		t.Fatalf("got %d orders, want 2", len(orders))
	}
	if len(orders[0].Result) != 0 || len(orders[1].Result) != 1 {
		t.Fatalf("result attached to wrong order: %d, %d", len(orders[0].Result), len(orders[1].Result))
	}
}

func TestDecodeErrorOmitsData(t *testing.T) {
	list := []struct {
		Name   string
		Input  string
		Secret string
	}{
		{"password", "H|\\^&||SECRETPW||||||||P||20221004013849\rL|1|N\r", "SECRETPW"},
		{"field value", testHeader + "P|1||||DOE^JOHN||19700101|M^X\rL|1|N\r", "M^X"},
		{"date of birth", testHeader + "P|1||||DOE^JOHN||1970-01-01\rL|1|N\r", "1970"},
		{"unknown segment", testHeader + "P|1||||DOE\nJOHN|SMITH\rL|1|N\r", "JOHN"},
	}
	for _, item := range list {
		t.Run(item.Name, func(t *testing.T) {
			_, err := decodeNoPanic(t, nil, item.Input)
			if err == nil {
				t.Fatal("expected error")
			}
			if strings.Contains(err.Error(), item.Secret) {
				t.Fatalf("error contains data %q: %v", item.Secret, err)
			}
		})
	}
}

func TestParseDateTime(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip(err)
	}
	list := []struct {
		In   string
		Loc  *time.Location
		Want time.Time
		Err  bool
	}{
		{In: "", Want: time.Time{}},
		{In: "2020", Want: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)},
		{In: "202005", Want: time.Date(2020, 5, 1, 0, 0, 0, 0, time.UTC)},
		{In: "20200522", Want: time.Date(2020, 5, 22, 0, 0, 0, 0, time.UTC)},
		{In: "2020052214", Want: time.Date(2020, 5, 22, 14, 0, 0, 0, time.UTC)},
		{In: "202005221438", Want: time.Date(2020, 5, 22, 14, 38, 0, 0, time.UTC)},
		{In: "20200522143859", Want: time.Date(2020, 5, 22, 14, 38, 59, 0, time.UTC)},
		{In: "20200522143859.198", Want: time.Date(2020, 5, 22, 14, 38, 59, 198e6, time.UTC)},
		{In: "20200522143859198", Want: time.Date(2020, 5, 22, 14, 38, 59, 198e6, time.UTC)},
		{In: "20200522143859198-0700", Want: time.Date(2020, 5, 22, 21, 38, 59, 198e6, time.UTC)},
		{In: "20200522143859+0130", Want: time.Date(2020, 5, 22, 13, 8, 59, 0, time.UTC)},
		{In: "2020-0700", Want: time.Date(2020, 1, 1, 7, 0, 0, 0, time.UTC)},
		{In: "20200522-0700", Want: time.Date(2020, 5, 22, 7, 0, 0, 0, time.UTC)},
		{In: "20200522143859", Loc: ny, Want: time.Date(2020, 5, 22, 18, 38, 59, 0, time.UTC)},
		{In: "20200522143859-0700", Loc: ny, Want: time.Date(2020, 5, 22, 21, 38, 59, 0, time.UTC)},

		{In: "2020-01-01", Err: true},
		{In: "1+", Err: true},
		{In: "1-34567890123456789", Err: true},
		{In: "202", Err: true},
		{In: "2020052", Err: true},
		{In: "20201301", Err: true},
		{In: "20200522143859-07", Err: true},
		{In: "202005221438.5", Err: true},
		{In: "20200522143859.", Err: true},
		{In: "20200522143859.1234567890", Err: true},
		{In: "٢٠٢٠٠٥٢٢", Err: true},
	}
	for _, item := range list {
		got, err := parseDateTime(item.In, item.Loc)
		if item.Err {
			if err == nil {
				t.Errorf("%q: expected error, got %v", item.In, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: %v", item.In, err)
			continue
		}
		if !got.Equal(item.Want) {
			t.Errorf("%q: got %v, want %v", item.In, got, item.Want)
		}
	}
}

func FuzzDecode(f *testing.F) {
	for _, fn := range []string{"lis2a.txt", "esr.txt"} {
		b, err := os.ReadFile(filepath.Join("testdata", fn))
		if err != nil {
			f.Fatal(err)
		}
		f.Add(b)
	}
	f.Add([]byte(testHeader + "P|1\rO|1|S1\rR|1|^^^X|1\rC|1|I|a^b\rL|1|N\r"))
	f.Add([]byte(testHeader + "M|1\rP|1||||A^B||20200522143859.198-0700\rL|1|N\r"))
	f.Add([]byte(testHeader + "P|1||||\x93Ko\x9ea\x94^M\xfcller\rC|1|L|caf\xe9 \x80\rL|1|N\r"))
	f.Add([]byte(testHeader + "P|1||||M\xc3\xbcller^\xe6\x9d\x8e\rL|1|N\r"))
	f.Fuzz(func(t *testing.T, b []byte) {
		for _, opt := range []*DecodeOption{nil, {LenientGroup: true}} {
			v, err := NewDecoder(lis2a2.Registry, opt).Decode(b)
			if err != nil {
				continue
			}
			out, err := NewEncoder(nil).Encode(v)
			if err != nil {
				continue
			}
			// Anything the encoder writes must decode again.
			if _, err := NewDecoder(lis2a2.Registry, opt).Decode(out); err != nil {
				t.Fatalf("re-decode of encoded output failed: %v\ninput: %q\noutput: %q", err, b, out)
			}
		}
	})
}
