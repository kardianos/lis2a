// Package lis2a Implements encoding and decoding LIS2-A and LIS2-A2 file format.
package lis2a

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

type RegistryLookup = map[string]any

type Registry interface {
	Version() string
	ControlSegment() RegistryLookup
	Segment() RegistryLookup
	Trigger() RegistryLookup
	DataType() RegistryLookup
}

const tagName = "lis2a"

var empty []any
var anyType = reflect.TypeOf(empty).Elem()

type structType byte

const (
	structUnknown structType = iota
	structTrigger
	structTriggerGroup // Trigger Sub-Type
	structSegment
	structDataType
)

type tag struct {
	Order      int32
	Name       string
	Format     string
	Type       structType
	Meta       bool
	Child      bool
	Omit       bool
	NoEscape   bool
	Sequence   bool
	FieldSep   bool
	FieldChars bool
	Present    bool
}

const lis2aMetaName = "LIS2A"

func parseTag(fieldName, v string) (tag, error) {
	t := tag{}
	if len(v) == 0 {
		return t, nil
	}
	t.Present = true
	ss := strings.Split(v, ",")
	s0 := ss[0]
	sN := ss[1:]
	if len(s0) > 0 {
		i, err := strconv.ParseInt(s0, 10, 32)
		if err != nil {
			return t, fmt.Errorf("field %q: unable to parse tag position: %w", fieldName, err)
		}
		t.Order = int32(i)
	}
	switch fieldName {
	case lis2aMetaName:
		t.Meta = true
	}
	for _, vv := range sN {
		k, v, _ := strings.Cut(vv, "=")

		switch k {
		default:
			return t, fmt.Errorf("field %q: unknown tag value %q", fieldName, vv)
		case "name":
			t.Name = v
		case "type":
			switch v {
			default:
				return t, fmt.Errorf("field %q: unknown type tag value %q", fieldName, vv)
			case "t":
				t.Type = structTrigger
			case "tg":
				t.Type = structTriggerGroup
			case "s":
				t.Type = structSegment
			case "d":
				t.Type = structDataType
			}
		case "format":
			t.Format = v
		case "child":
			t.Child = true
		case "noescape":
			t.NoEscape = true
		case "omit":
			t.Omit = true
		case "seq":
			t.Sequence = true
		case "required":
			// TODO.
		case "conditional":
			// TODO.
		case "len":
			// TODO.
		case "max":
			// TODO.
		case "display":
			// TODO.
		case "table":
			// TODO.
		case "fieldsep":
			t.FieldSep = true
		case "fieldchars":
			t.FieldChars = true
		}
	}
	return t, nil
}
