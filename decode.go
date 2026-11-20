package lis2a

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"
)

type lineDecoder struct {
	sep      byte    // usually a |
	repeat   byte    // usually a \
	dividers [2]byte // usually "|^"
	chars    [3]byte // usually "\^&"
	escape   byte    // usually a "&"
	readSep  bool
	charset  Charset // Resolved for the current message, never CharsetDefault.

	unescaper *strings.Replacer

	opt DecodeOption
}

// Decoder decodes bytes into LIS2A structures.
//
// A Decoder holds no per-message state and may be used concurrently.
type Decoder struct {
	registry Registry
	opt      DecodeOption
}

// DecodeOption sets options for the LIS2A decoder.
type DecodeOption struct {
	ErrorZSegment bool // Error on an unknown Zxx segment when true.

	// LenientGroup accepts segments that arrive out of order, or without the
	// segment that starts their group (for example an R record with no
	// preceding O record), creating empty groups as needed.
	// When false (the default) these return an error.
	LenientGroup bool

	// Location is used for date-times that do not include a UTC offset.
	// If nil, UTC is used.
	Location *time.Location

	// Charset of the message. Decoded strings are always UTF-8.
	// The default decodes valid UTF-8 as UTF-8 and anything else as Windows-1252.
	Charset Charset
}

// NewDecoder creates a new Decoder. A registry must be provided. Option is optional.
func NewDecoder(registry Registry, opt *DecodeOption) *Decoder {
	d := &Decoder{
		registry: registry,
	}
	if opt != nil {
		d.opt = *opt
	}

	return d
}

// Decode takes a single lis2a message and returns a final trigger with all segments grouped.
//
// The entire message is held in memory; callers reading from a network or
// device should limit the message size they accept.
func (d *Decoder) Decode(data []byte) (any, error) {
	list, err := d.DecodeList(data)
	if err != nil {
		return nil, fmt.Errorf("segment list: %w", err)
	}
	g, err := d.DecodeGroup(list)
	if err != nil {
		return nil, fmt.Errorf("trigger group: %w", err)
	}
	return g, nil
}

// DecodeGroup groups a list of segments into trigger groupings.
func (d *Decoder) DecodeGroup(list []any) (any, error) {
	return group(list, d.registry, d.opt.LenientGroup)
}

// DecodeList returns a list of segments without any grouping applied.
func (d *Decoder) DecodeList(data []byte) ([]any, error) {
	// Explicitly accept both CR and LF as new lines. Some systems do use \n, despite the spec.
	lines := bytes.FieldsFunc(data, func(r rune) bool {
		switch r {
		default:
			return false
		case '\r', '\n':
			return true
		}
	})

	type field struct {
		name  string
		index int
		tag   tag
		field reflect.Value
	}

	ret := []any{}

	ld := &lineDecoder{opt: d.opt, charset: d.opt.Charset.resolve(data)}
	segmentRegistry := d.registry.Segment()
	for index, line := range lines {
		lineNumber := index + 1
		if len(line) == 0 {
			continue
		}
		if ld.charset == CharsetUTF8 && !utf8.Valid(line) {
			return nil, fmt.Errorf("line %d: invalid UTF-8", lineNumber)
		}

		segTypeName, n := ld.getID(line)
		remain := line[n:]
		if len(segTypeName) == 0 {
			return nil, fmt.Errorf("line %d: missing segment type", lineNumber)
		}

		seg, ok := segmentRegistry[segTypeName]
		if !ok {
			isZ := segTypeName[0] == 'Z'
			if isZ && !d.opt.ErrorZSegment {
				continue
			}
			return nil, fmt.Errorf("line %d: unknown segment type %s", lineNumber, safeSegmentName(segTypeName))
		}

		rt := reflect.TypeOf(seg)
		if rt == nil || rt.Kind() != reflect.Struct {
			return nil, fmt.Errorf("line %d: segment %q registered with non-struct type %v", lineNumber, segTypeName, rt)
		}
		ct := rt.NumField()

		fieldList := make([]field, 0, ct)

		hasInit := false

		rv := reflect.New(rt)
		rvv := rv.Elem()

		var SegmentName string
		var SegmentSize int32
		var maxOrd int32

		for i := 0; i < ct; i++ {
			ft := rt.Field(i)
			tagText := ft.Tag.Get(tagName)
			tag, err := parseTag(ft.Name, tagText)
			if err != nil {
				return nil, err
			}
			if !tag.Present {
				continue
			}
			if tag.Meta {
				SegmentName = tag.Name
				SegmentSize = tag.Order
				if ft.Type.Kind() == reflect.String {
					rvv.Field(i).SetString(tag.Name)
				}
				continue
			}
			if tag.Order > maxOrd {
				maxOrd = tag.Order
			}
			if tag.FieldSep || tag.FieldChars {
				hasInit = true
			}
			f := field{
				name:  ft.Name,
				index: i,
				tag:   tag,
			}
			f.field = rvv.Field(i)

			if !f.field.IsValid() {
				return nil, fmt.Errorf("%s.%s invalid reflect value", SegmentName, f.name)
			}

			fieldList = append(fieldList, f)
		}
		if SegmentSize == 0 {
			SegmentSize = maxOrd
		}
		SegmentFieldLength := int(SegmentSize + 1)

		offset := 0
		if hasInit {
			if len(remain) < 4 {
				return nil, fmt.Errorf("line %d: missing format delimiters", lineNumber)
			}
			if err := validDelimiters(remain[0], remain[1:4]); err != nil {
				return nil, fmt.Errorf("line %d: %w", lineNumber, err)
			}
			ld.sep = remain[0]
			copy(ld.chars[:], remain[1:4])

			ld.dividers = [2]byte{ld.sep, ld.chars[1]}
			ld.repeat = ld.chars[0]
			ld.escape = ld.chars[2]
			ld.setupUnescaper()
			ld.readSep = true

			remain = remain[4:]
			offset = 2
		}

		if ld.sep == 0 {
			return nil, fmt.Errorf("line %d: missing separator prior to field", lineNumber)
		}

		parts := bytes.Split(remain, []byte{ld.sep})

		ff := make([]field, SegmentFieldLength)
		for _, f := range fieldList {
			if f.tag.FieldSep {
				f.field.SetString(string(ld.sep))
				continue
			}
			if f.tag.FieldChars {
				f.field.SetString(string(ld.chars[:]))
				continue
			}
			index := int(f.tag.Order) - offset
			if index < 0 || index >= SegmentFieldLength {
				continue
			}
			ff[index] = f
		}

		for i, f := range ff {
			if i >= len(parts) {
				break
			}
			p := parts[i]
			if !f.tag.Present {
				continue
			}
			if f.tag.Omit {
				continue
			}
			if f.tag.Child {
				continue
			}
			err := ld.decodeSegmentList(p, f.tag, f.field)
			if err != nil {
				return nil, fmt.Errorf("line %d, %s.%s: %w", lineNumber, SegmentName, f.name, err)
			}
		}

		ret = append(ret, rv.Interface())
	}
	return ret, nil
}

// safeSegmentName returns a segment name for an error message.
// Long names are likely field data from a malformed line and are not included.
func safeSegmentName(name string) string {
	if len(name) <= 3 {
		return fmt.Sprintf("%q", name)
	}
	return fmt.Sprintf("(%d bytes)", len(name))
}

// validDelimiters ensures the field separator and encoding characters
// are printable, not alphanumeric, and distinct from each other.
func validDelimiters(sep byte, chars []byte) error {
	if len(chars) != 3 {
		return fmt.Errorf("encoding characters must be 3 bytes, got %d", len(chars))
	}
	all := [4]byte{sep, chars[0], chars[1], chars[2]}
	for i, c := range all {
		if c <= ' ' || c >= 0x7f || ('0' <= c && c <= '9') || ('A' <= c && c <= 'Z') || ('a' <= c && c <= 'z') {
			return fmt.Errorf("invalid delimiter %q", c)
		}
		for _, o := range all[:i] {
			if c == o {
				return fmt.Errorf("delimiter %q used more than once", c)
			}
		}
	}
	return nil
}

func (d *lineDecoder) setupUnescaper() {
	d.unescaper = strings.NewReplacer(
		string([]byte{d.escape, 'F', d.escape}), string(d.sep),
		string([]byte{d.escape, 'S', d.escape}), string(d.chars[1]),
		string([]byte{d.escape, 'R', d.escape}), string(d.chars[0]),
		string([]byte{d.escape, 'E', d.escape}), string(d.chars[2]),
	)
}

var timeType reflect.Type = reflect.TypeOf(time.Time{})

func (d *lineDecoder) decodeSegmentList(data []byte, t tag, rv reflect.Value) error {
	if len(data) == 0 {
		return nil
	}
	parts := bytes.Split(data, []byte{d.repeat})
	for _, p := range parts {
		if len(p) == 0 {
			continue
		}
		err := d.decodeSegment(p, t, rv, 1, len(parts) > 1)
		if err != nil {
			return fmt.Errorf("%s.%d: %w", rv.Type().String(), t.Order, err)
		}
	}
	return nil
}

func (d *lineDecoder) decodeSegment(data []byte, t tag, rv reflect.Value, level int, mustBeSlice bool) error {
	type field struct {
		tag   tag
		field reflect.Value
	}

	if !rv.IsValid() {
		// No field defined at this position.
		return nil
	}

	isSlice := rv.Kind() == reflect.Slice
	if mustBeSlice && !isSlice {
		return fmt.Errorf("data repeats but element %v does not", rv.Type())
	}

	switch rv.Kind() {
	default:
		return fmt.Errorf("unknown field kind %v (%v)", rv.Kind(), rv.Type())
	case reflect.Interface:
		// TODO: Support a true VARIES.
		return fmt.Errorf("unsupported interface field kind")
	case reflect.Pointer:
		next := reflect.New(rv.Type().Elem())
		rv.Set(next)
		return d.decodeSegment(data, t, next.Elem(), level, false)
	case reflect.Slice:
		if len(data) == 0 {
			return nil
		}
		itemType := rv.Type().Elem()
		itemValue := reflect.New(itemType)
		ivv := itemValue.Elem()
		err := d.decodeSegment(data, t, ivv, level, false)
		if err != nil {
			return fmt.Errorf("slice: %w", err)
		}

		rv.Set(reflect.Append(rv, ivv))
		return nil
	case reflect.Struct:
		switch rv.Type() {
		default:
			if level >= len(d.dividers) {
				return fmt.Errorf("%v: components may not be nested more than one level", rv.Type())
			}
			sep := d.dividers[level]

			rt := rv.Type()
			ct := rv.NumField()

			fieldList := []field{}

			var SegmentName string
			var SegmentSize int32
			var maxOrd int32

			for i := 0; i < ct; i++ {
				ft := rt.Field(i)
				fTag, err := parseTag(ft.Name, ft.Tag.Get(tagName))
				if err != nil {
					return err
				}

				if fTag.Meta {
					SegmentName = fTag.Name
					SegmentSize = fTag.Order
					if ft.Type.Kind() == reflect.String {
						rv.Field(i).SetString(SegmentName)
					}
					continue
				}
				if !fTag.Present {
					continue
				}
				if fTag.Omit {
					continue
				}
				if fTag.Child {
					continue
				}
				if fTag.Order > maxOrd {
					maxOrd = fTag.Order
				}

				fv := rv.Field(i)

				f := field{
					tag:   fTag,
					field: fv,
				}
				fieldList = append(fieldList, f)
			}
			if SegmentSize == 0 {
				SegmentSize = maxOrd
			}
			ff := make([]field, int(SegmentSize))

			for _, f := range fieldList {
				index := int(f.tag.Order - 1)
				if index < 0 || index >= len(ff) {
					continue
				}

				ff[index] = f
			}

			// TODO: Make more robust. Watch for repeats, etc, other stuff.
			parts := bytes.Split(data, []byte{sep})
			for i, p := range parts {
				if i >= len(ff) {
					continue
				}
				f := ff[i]
				if !f.field.IsValid() {
					continue
				}
				err := d.decodeSegment(p, f.tag, f.field, level+1, false)
				if err != nil {
					return fmt.Errorf("%s-%s.%d: %w", SegmentName, f.field.Type().String(), f.tag.Order, err)
				}
			}
			return nil
		case timeType:
			v := d.decodeByte(data, t)
			t, err := parseDateTime(v, d.opt.Location)
			if err != nil {
				return err
			}
			rv.Set(reflect.ValueOf(t))
			return nil
		}
	case reflect.String:
		c1, c2 := d.dividers[0], d.dividers[1]
		for _, b := range data {
			switch b {
			case c1, c2:
				return fmt.Errorf("%s contains a delimiter character; data may be malformed, invalid type, or contain a bug", rv.Type())
			}
		}
		rv.SetString(d.decodeByte(data, t))
		return nil
	}
}

func (d *lineDecoder) decodeByte(v []byte, t tag) string {
	// Escape sequences and delimiters are ASCII, so convert before unescaping.
	s := decodeText(v, d.charset)
	if t.NoEscape {
		return s
	}
	return d.unescaper.Replace(s)
}

func (d *lineDecoder) getID(data []byte) (string, int) {
	if d.readSep {
		v, _, _ := bytes.Cut(data, []byte{d.sep})
		return string(v), len(v)
	}
	for i, c := range data {
		if ('0' <= c && c <= '9') || ('A' <= c && c <= 'Z') || ('a' <= c && c <= 'z') {
			continue
		}
		return string(data[:i]), i
	}
	return string(data), len(data)
}

// Layouts for a date-time by the number of leading digits.
var dateTimeLayout = map[int]string{
	4:  "2006",
	6:  "200601",
	8:  "20060102",
	10: "2006010215",
	12: "200601021504",
	14: "20060102150405",
}

// parseDateTime parses a date-time in the form:
//
//	YYYY[MM[DD[HH[MM[SS[[.]S[S[S[S]]]]]]]]][+/-ZZZZ]
//
// Fractional seconds may be written with or without the decimal point.
// If no UTC offset is given, loc is used (UTC if loc is nil).
// Errors do not include the value, as it may be patient data such as a date of birth.
func parseDateTime(dt string, loc *time.Location) (time.Time, error) {
	if len(dt) == 0 {
		return time.Time{}, nil // No date supplied, use zero value.
	}
	if loc == nil {
		loc = time.UTC
	}

	main, zone := dt, ""
	if i := strings.LastIndexAny(dt, "+-"); i >= 0 {
		main, zone = dt[:i], dt[i:]
		if len(zone) != 5 || !isDigits(zone[1:]) {
			return time.Time{}, fmt.Errorf("invalid date-time: UTC offset must be in the form +/-ZZZZ")
		}
	}

	frac := ""
	if i := strings.IndexByte(main, '.'); i >= 0 {
		main, frac = main[:i], main[i+1:]
		if len(main) != 14 || len(frac) == 0 {
			return time.Time{}, fmt.Errorf("invalid date-time: fractional seconds require a full date and time")
		}
	} else if len(main) > 14 {
		main, frac = main[:14], main[14:]
	}
	if !isDigits(main) || !isDigits(frac) {
		return time.Time{}, fmt.Errorf("invalid date-time: unexpected characters")
	}
	if len(frac) > 9 {
		return time.Time{}, fmt.Errorf("invalid date-time: too many fractional second digits")
	}
	layout, ok := dateTimeLayout[len(main)]
	if !ok {
		return time.Time{}, fmt.Errorf("invalid date-time: %d digits is not a valid precision", len(main))
	}

	var t time.Time
	var err error
	if len(zone) > 0 {
		t, err = time.Parse(layout+"-0700", main+zone)
	} else {
		t, err = time.ParseInLocation(layout, main, loc)
	}
	if err != nil {
		// Do not wrap the time error, it includes the input value.
		return time.Time{}, fmt.Errorf("invalid date-time: out of range value")
	}
	if len(frac) > 0 {
		ns := 0
		for i := 0; i < 9; i++ {
			ns *= 10
			if i < len(frac) {
				ns += int(frac[i] - '0')
			}
		}
		t = t.Add(time.Duration(ns))
	}
	return t, nil
}

func isDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
