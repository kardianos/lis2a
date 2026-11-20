package lis2a

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/kardianos/lis2a/lis2a2"
)

func testMessage(name string) lis2a2.LIS2_A2 {
	return lis2a2.LIS2_A2{
		Header: lis2a2.H{MessageVersion: "NCCLS LIS2-A"},
		Patient: []lis2a2.LIS2_A2_P{{
			P: lis2a2.P{PatientName: lis2a2.Name{Last: name}},
			Order: []lis2a2.LIS2_A2_O{{
				Order:  lis2a2.O{ID: lis2a2.SpecimenID{ID: "S1"}},
				Result: []lis2a2.LIS2_A2_R{{Result: lis2a2.R{Value: lis2a2.DataValue{Value: "5"}}}},
			}},
		}},
		Last: lis2a2.L{Code: "N"},
	}
}

func encodeNoPanic(t *testing.T, opt *EncodeOption, msg any) (out []byte, err error) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic: %v", r)
		}
	}()
	return NewEncoder(opt).Encode(msg)
}

func TestEncodeRejectsControlCharacters(t *testing.T) {
	for _, v := range []string{"Doe\rO", "Doe\nL", "Doe\x00", "Doe\x7f", "Doe\tX"} {
		m := testMessage(v)
		_, err := encodeNoPanic(t, nil, m)
		if err == nil {
			t.Errorf("%q: expected error", v)
		}
		m = testMessage("DOE")
		m.Patient[0].Order[0].Comment = []lis2a2.C{{Source: "L", Text: lis2a2.CommentText{C1: v}}}
		_, err = encodeNoPanic(t, nil, m)
		if err == nil {
			t.Errorf("comment %q: expected error", v)
		}
	}
}

func TestEncodeRepeat(t *testing.T) {
	m := testMessage("DOE")
	m.Patient[0].Order[0].Order.Priority = []string{"S", "R"}
	out, err := encodeNoPanic(t, &EncodeOption{TrimTrailingSeparator: true}, m)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out, []byte(`|S\R`)) {
		t.Fatalf("repeat not encoded with repeat delimiter: %q", out)
	}
	v, err := decodeNoPanic(t, nil, string(out))
	if err != nil {
		t.Fatal(err)
	}
	got := v.(lis2a2.LIS2_A2).Patient[0].Order[0].Order.Priority
	if len(got) != 2 || got[0] != "S" || got[1] != "R" {
		t.Fatalf("got %q", got)
	}
}

func TestEncodeEscape(t *testing.T) {
	const name = `A|B^C\D&E`
	out, err := encodeNoPanic(t, nil, testMessage(name))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out, []byte(`A&F&B&S&C&R&D&E&E`)) {
		t.Fatalf("not escaped: %q", out)
	}
	v, err := decodeNoPanic(t, nil, string(out))
	if err != nil {
		t.Fatal(err)
	}
	if got := v.(lis2a2.LIS2_A2).Patient[0].P.PatientName.Last; got != name {
		t.Fatalf("got %q, want %q", got, name)
	}
}

func TestEncodeReturnsCopy(t *testing.T) {
	enc := NewEncoder(nil)
	a, err := enc.Encode(testMessage("ALICE"))
	if err != nil {
		t.Fatal(err)
	}
	before := string(a)
	if _, err := enc.Encode(testMessage("BOB")); err != nil {
		t.Fatal(err)
	}
	if string(a) != before {
		t.Fatal("first result modified by second Encode")
	}
}

func TestEncodeDelimiters(t *testing.T) {
	t.Run("custom", func(t *testing.T) {
		m := testMessage("DOE^JOHN")
		m.Header.FieldSeparator = "!"
		m.Header.EncodingCharacters = `~^#`
		out, err := encodeNoPanic(t, &EncodeOption{TrimTrailingSeparator: true}, m)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.HasPrefix(out, []byte(`H!~^#!`)) || !bytes.Contains(out, []byte("P!1!!!!DOE#S#JOHN")) {
			t.Fatalf("custom delimiters not used: %q", out)
		}
		v, err := decodeNoPanic(t, nil, string(out))
		if err != nil {
			t.Fatal(err)
		}
		if got := v.(lis2a2.LIS2_A2).Patient[0].P.PatientName.Last; got != "DOE^JOHN" {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("default", func(t *testing.T) {
		out, err := encodeNoPanic(t, nil, testMessage("DOE"))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.HasPrefix(out, []byte(`H|\^&|`)) {
			t.Fatalf("default delimiters not written: %q", out)
		}
	})
	for _, chars := range []string{`\`, `\^`, `\^&~`, `\\&`, `\^|`, `a^&`, "\r^&"} {
		m := testMessage("DOE")
		m.Header.EncodingCharacters = chars
		if _, err := encodeNoPanic(t, nil, m); err == nil {
			t.Errorf("EncodingCharacters %q: expected error", chars)
		}
	}
}

func TestEncodeTime(t *testing.T) {
	in := testHeader + "P|1||||DOE||19700101\rO|1|S1||||||||||||20221001013247-0700\rL|1|N\r"
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip(err)
	}

	t.Run("location round trip", func(t *testing.T) {
		v, err := decodeNoPanic(t, &DecodeOption{Location: ny}, in)
		if err != nil {
			t.Fatal(err)
		}
		want := v.(lis2a2.LIS2_A2).Patient[0].Order[0].Order.ReceivedDateTime
		out, err := encodeNoPanic(t, &EncodeOption{Location: ny}, v)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(out, []byte("|20221001043247|")) {
			t.Fatalf("time not converted to location: %q", out)
		}
		if !bytes.Contains(out, []byte("|19700101|")) {
			t.Fatalf("date of birth changed: %q", out)
		}
		v2, err := decodeNoPanic(t, &DecodeOption{Location: ny}, string(out))
		if err != nil {
			t.Fatal(err)
		}
		if got := v2.(lis2a2.LIS2_A2).Patient[0].Order[0].Order.ReceivedDateTime; !got.Equal(want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})
	t.Run("zone offset", func(t *testing.T) {
		v, err := decodeNoPanic(t, nil, in)
		if err != nil {
			t.Fatal(err)
		}
		want := v.(lis2a2.LIS2_A2).Patient[0].Order[0].Order.ReceivedDateTime
		out, err := encodeNoPanic(t, &EncodeOption{ZoneOffset: true}, v)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(out, []byte("|20221001013247-0700|")) || !bytes.Contains(out, []byte("|19700101|")) {
			t.Fatalf("unexpected time encoding: %q", out)
		}
		v2, err := decodeNoPanic(t, nil, string(out))
		if err != nil {
			t.Fatal(err)
		}
		if got := v2.(lis2a2.LIS2_A2).Patient[0].Order[0].Order.ReceivedDateTime; !got.Equal(want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})
}

func TestEncodeErrorOmitsData(t *testing.T) {
	_, err := encodeNoPanic(t, nil, testMessage("SECRET-NAME\r"))
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("error contains data: %v", err)
	}
}
