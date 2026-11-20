package lis2a

import (
	"bytes"
	"fmt"
	"reflect"
	"strconv"
	"time"
)

const nextLine = '\r'
const defaultSep = "|"
const defaultChars = `\^&`

// NewEncoder creates a new Encoder. Options may be nil.
func NewEncoder(opt *EncodeOption) *Encoder {
	e := &Encoder{}
	if opt != nil {
		e.opt = *opt
	}
	return e
}

// EncodeOption sets options for the LIS2A encoder.
type EncodeOption struct {
	TrimTrailingSeparator bool

	// Location, if set, converts date-times to this location before they are written.
	// Applies to date-time formats (YMDHMS, YMDHM and the default);
	// date-only (YMD) and time-only (HM) values are written as-is.
	Location *time.Location

	// ZoneOffset appends the UTC offset (+/-ZZZZ) to date-time values.
	// Applies to the same formats as Location.
	ZoneOffset bool

	// Charset to write. The default is Windows-1252.
	// Characters the charset cannot represent are an error.
	Charset Charset
}

// Encoder encodes LIS2A structures into bytes.
//
// An Encoder reuses internal buffers between calls and must not be used concurrently.
type Encoder struct {
	sep      byte    // usually a |
	chars    [3]byte // usually \^&
	repeat   byte    // usually a \
	dividers []byte
	esc      map[byte][]byte

	deferred [3]*bytes.Buffer
	buf      *bytes.Buffer
	text     []byte // Scratch space for converting a value to the output charset.

	opt EncodeOption
}

// Encode a message. The returned slice is owned by the caller.
func (e *Encoder) Encode(message any) ([]byte, error) {
	e.reset()
	err := e.setDelimiters("", "")
	if err != nil {
		return nil, err
	}

	err = e.walk(1, reflect.ValueOf(message))
	if err != nil {
		return nil, err
	}
	// Write the record terminator of the final segment.
	e.flushDeferred(0)
	return append([]byte(nil), e.buf.Bytes()...), nil
}

// reset the output buffers.
func (e *Encoder) reset() {
	for i := range e.deferred {
		if e.deferred[i] == nil {
			e.deferred[i] = &bytes.Buffer{}
		}
		e.deferred[i].Reset()
	}
	if e.buf == nil {
		e.buf = &bytes.Buffer{}
	}
	e.buf.Reset()
}

// setDelimiters sets the field separator and encoding characters.
// If sep or chars are empty, the default is used.
func (e *Encoder) setDelimiters(sep, chars string) error {
	if len(sep) == 0 {
		sep = defaultSep
	}
	if len(chars) == 0 {
		chars = defaultChars
	}
	if len(sep) != 1 {
		return fmt.Errorf("field separator must be 1 byte, got %d", len(sep))
	}
	if err := validDelimiters(sep[0], []byte(chars)); err != nil {
		return err
	}

	e.sep = sep[0]
	copy(e.chars[:], chars)
	e.repeat = chars[0]
	e.dividers = []byte{sep[0], chars[1]}

	esc := chars[2]
	e.esc = map[byte][]byte{
		e.sep:    {esc, 'F', esc},
		chars[1]: {esc, 'S', esc},
		chars[0]: {esc, 'R', esc},
		chars[2]: {esc, 'E', esc},
	}
	return nil
}

func (e *Encoder) walk(seq int, wv reflect.Value) error {
	if !wv.IsValid() {
		return nil
	}
	metaTag, err := e.meta(wv.Type())
	if err != nil {
		return err
	}
	if !metaTag.Present {
		return nil
	}
	switch metaTag.Type {
	default:
		return nil
	case structTrigger, structTriggerGroup:
		switch wv.Kind() {
		default:
			return nil
		case reflect.Pointer:
			return e.walk(seq, wv.Elem())
		case reflect.Slice:
			l := wv.Len()
			for i := 0; i < l; i++ {
				v := wv.Index(i)
				err := e.walk(i+1, v)
				if err != nil {
					return err
				}
			}
			return nil
		case reflect.Struct:
			ct := wv.NumField()
			for i := 0; i < ct; i++ {
				v := wv.Field(i)
				err := e.walk(seq, v)
				if err != nil {
					return err
				}
			}
			return nil
		}
	case structSegment:
		switch wv.Kind() {
		default:
			return nil
		case reflect.Pointer:
			return e.walk(seq, wv.Elem())
		case reflect.Slice:
			l := wv.Len()
			for i := 0; i < l; i++ {
				v := wv.Index(i)
				err := e.encodeSegment(i+1, v)
				if err != nil {
					return err
				}
			}
			return nil
		case reflect.Struct:
			return e.encodeSegment(seq, wv)
		}
	}
}
func (e *Encoder) meta(wt reflect.Type) (tag, error) {
	switch wt.Kind() {
	default:
		return tag{}, nil
	case reflect.Pointer:
		return e.meta(wt.Elem())
	case reflect.Slice:
		return e.meta(wt.Elem())
	case reflect.Struct:
		sf, ok := wt.FieldByName(lis2aMetaName)
		if !ok {
			return tag{}, nil
		}
		return parseTag(sf.Name, sf.Tag.Get(tagName))
	}
}

// encodeSegment the given message into buffer.
func (e *Encoder) encodeSegment(seq int, st reflect.Value) error {
	stt := st.Type()

	var SegmentName string
	var SegmentSize int32
	var maxOrd int32

	type field struct {
		name    string
		present bool
		tag     tag
		value   any
	}
	var fieldList []field

	var msgSep, msgChars string
	var hasDelimiters bool
	for i := 0; i < st.NumField(); i++ {
		fld := stt.Field(i)
		f := st.Field(i)
		tag, err := parseTag(fld.Name, fld.Tag.Get(tagName))
		if err != nil {
			return err
		}
		switch {
		case tag.FieldSep:
			msgSep = f.String()
			hasDelimiters = true
		case tag.FieldChars:
			msgChars = f.String()
			hasDelimiters = true
		}

		if tag.Meta {
			switch tag.Type {
			case structTrigger, structTriggerGroup:
				return fmt.Errorf("trigger and trigger group structures should not be passed in to encode, package error")
			}
		}
		if !tag.Present {
			continue
		}
		if tag.Meta {
			SegmentName = tag.Name
			SegmentSize = tag.Order
		} else {
			if tag.Order > maxOrd {
				maxOrd = tag.Order
			}
		}

		fieldList = append(fieldList, field{
			name:    fld.Name,
			present: !f.IsZero(),
			tag:     tag,
			value:   f.Interface(),
		})
	}
	if hasDelimiters {
		if err := e.setDelimiters(msgSep, msgChars); err != nil {
			return fmt.Errorf("%s: %w", SegmentName, err)
		}
	}

	if SegmentSize == 0 {
		SegmentSize = maxOrd
	}
	ff := make([]field, SegmentSize)
	for _, f := range fieldList {
		index := f.tag.Order - 1
		if index < 0 || index >= SegmentSize {
			continue
		}
		ff[index] = f
	}

	if err := e.write(SegmentName, 0, true); err != nil {
		return err
	}
	for _, f := range ff {
		if f.tag.Omit {
			continue
		}
		v := f.value
		switch {
		// Auto populate SetID sequences.
		case f.tag.Sequence:
			if seq == 0 {
				return fmt.Errorf("incorrect zero sequence number")
			}
			if s, ok := v.(string); ok && len(s) == 0 {
				v = strconv.FormatInt(int64(seq), 10)
			}
		}
		direct := !e.opt.TrimTrailingSeparator
		e.writeSep(0, 0, direct)
		if f.tag.FieldChars {
			// Write the encoding characters in effect, already validated by setDelimiters.
			if err := e.write(string(e.chars[:]), 0, true); err != nil {
				return err
			}
			continue
		}
		err := e.encodeDataType(f.tag, v, 0)
		if err != nil {
			return fmt.Errorf("%s.%s: %w", SegmentName, f.name, err)
		}
	}
	e.resetAllDeferred()
	e.writeSep(0, nextLine, false)
	return nil
}

func (e *Encoder) flushDeferred(level int) {
	// If level 0"|", then write level 0, remove 1, 2.
	// If level 1"^", then write level 0 and 1, remove 2.
	// If level 2"&", then write level 0, 1, and 2.
	for index, d := range e.deferred {
		if index <= level {
			db := d.Bytes()
			e.buf.Write(db)
		}
		d.Reset()
	}
}

// write converts individual values to the output charset and escapes them.
//
// Control characters (including CR and LF, which end a record) cannot be
// escaped and are rejected, even when noEscape is set.
func (e *Encoder) write(val string, level int, noEscape bool) error {
	text, err := appendText(e.text[:0], val, e.opt.Charset)
	e.text = text
	if err != nil {
		return err
	}
	if len(text) > 0 {
		e.flushDeferred(level)
	}
	buf := e.buf
	if noEscape {
		buf.Write(text)
		return nil
	}
	// Delimiters are ASCII and never part of a multi-byte character.
	for _, c := range text {
		if esc, is := e.esc[c]; is {
			buf.Write(esc)
			continue
		}
		buf.WriteByte(c)
	}
	return nil
}
func (e *Encoder) resetAllDeferred() {
	for _, d := range e.deferred {
		d.Reset()
	}
}
func (e *Encoder) writeSep(level int, sep byte, direct bool) {
	if sep == 0 {
		sep = e.dividers[level]
	}
	// If level 0"|", then write it directly and reset the other layers.
	// If level 1"&", then reset 2 and write to defered 1.
	for index, d := range e.deferred {
		if index > level {
			d.Reset()
		}
	}
	if direct {
		e.deferred[level].WriteByte(sep)
		e.flushDeferred(level)
		return
	}
	e.deferred[level].WriteByte(sep)
}

func (e *Encoder) encodeDataType(t tag, o interface{}, level int) error {
	if o == nil || !t.Present {
		return nil
	}

	switch v := o.(type) {
	default:
		rv := reflect.ValueOf(o)
		if rv.IsZero() {
			return nil
		}
		switch rv.Kind() {
		default:
			return fmt.Errorf("unknown value kind: %v", rv.Kind())
		case reflect.Pointer:
			return e.encodeDataType(t, rv.Elem().Interface(), level)
		case reflect.Struct:
			if level+1 >= len(e.dividers) {
				return fmt.Errorf("%v: components may not be nested more than one level", rv.Type())
			}
			var SegmentName string
			var SegmentSize int32
			var maxOrd int32

			type field struct {
				name    string
				present bool
				tag     tag
				value   any
			}
			var fieldList []field

			rt := rv.Type()
			ct := rt.NumField()

			for i := 0; i < ct; i++ {
				ft := rt.Field(i)
				tag, err := parseTag(ft.Name, ft.Tag.Get(tagName))
				if err != nil {
					return err
				}

				if tag.Child {
					return fmt.Errorf("children cannot be on this segment")
				}
				if !tag.Present {
					continue
				}
				if tag.Meta {
					SegmentName = tag.Name
					SegmentSize = tag.Order
				} else {
					if tag.Order > maxOrd {
						maxOrd = tag.Order
					}
				}
				f := rv.Field(i)

				fieldList = append(fieldList, field{
					name:    ft.Name,
					present: !f.IsZero(),
					tag:     tag,
					value:   f.Interface(),
				})
			}

			if SegmentSize == 0 {
				SegmentSize = maxOrd
			}
			ff := make([]field, SegmentSize)
			for _, f := range fieldList {
				index := f.tag.Order - 1
				if index < 0 || index >= SegmentSize {
					continue
				}
				ff[index] = f
			}

			for i, f := range ff {
				if i != 0 {
					e.writeSep(level+1, 0, false)
				}
				err := e.encodeDataType(f.tag, f.value, level+1)
				if err != nil {
					return fmt.Errorf("%s.%s: %w", SegmentName, f.name, err)
				}
			}
		case reflect.Slice:
			ct := rv.Len()
			for i := 0; i < ct; i++ {
				if i != 0 {
					e.writeSep(level, e.repeat, true)
				}
				x := rv.Index(i)
				value := x.Interface()

				err := e.encodeDataType(t, value, level)
				if err != nil {
					return fmt.Errorf("repeat %d: %w", i+1, err)
				}
			}
			return nil
		}
	case string:
		return e.write(v, level, t.NoEscape)
	case time.Time:
		if v.IsZero() {
			return nil
		}
		var layout string
		dateTime := true
		switch t.Format {
		default:
			layout = "20060102150405"
		case "YMDHMS":
			layout = "20060102150405"
		case "YMDHM":
			layout = "200601021504"
		case "YMD":
			layout = "20060102"
			dateTime = false
		case "HM":
			layout = "1504"
			dateTime = false
		}
		if dateTime {
			if e.opt.Location != nil {
				v = v.In(e.opt.Location)
			}
			if e.opt.ZoneOffset {
				layout += "-0700"
			}
		}
		return e.write(v.Format(layout), level, t.NoEscape)
	}
	return nil
}
