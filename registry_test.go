package lis2a

import "testing"

// Registry shapes that previously caused panics.

type shapeName struct{}

type shapeInner struct {
	LIS2A shapeName `lis2a:",name=Inner,type=d"`
	A     string    `lis2a:"1"`
}
type shapeNested struct {
	LIS2A shapeName  `lis2a:",name=Nested,type=d"`
	In    shapeInner `lis2a:"1"`
}
type shapeGap struct {
	LIS2A shapeName `lis2a:",name=Gap,type=d"`
	A     string    `lis2a:"1"`
	C     string    `lis2a:"3"`
}
type shapeH struct {
	LIS2A shapeName `lis2a:",name=H,type=s"`
	FS    string    `lis2a:"1,fieldsep,omit"`
	FC    string    `lis2a:"2,fieldchars"`
	ID    string    `lis2a:"3"`
}

func (h shapeH) MessageStructureID() string { return h.ID }

type shapeX struct {
	LIS2A shapeName   `lis2a:",name=X,type=s"`
	N     shapeNested `lis2a:"1"`
	G     shapeGap    `lis2a:"2"`
}
type shapeRecursive struct {
	LIS2A shapeName        `lis2a:",name=Rec,type=tg"`
	H     shapeH           `lis2a:"1"`
	Sub   []shapeRecursive `lis2a:"2"`
}

type shapeRegistry struct{}

func (shapeRegistry) Version() string                { return "shape" }
func (shapeRegistry) ControlSegment() RegistryLookup { return nil }
func (shapeRegistry) Segment() RegistryLookup {
	return RegistryLookup{"H": shapeH{}, "X": shapeX{}, "S": "not a struct"}
}
func (shapeRegistry) Trigger() RegistryLookup  { return RegistryLookup{"REC": shapeRecursive{}} }
func (shapeRegistry) DataType() RegistryLookup { return nil }

func TestRegistryShapes(t *testing.T) {
	list := []struct {
		Name  string
		Input string
		Group bool
		Err   bool
	}{
		{"nested component", "H|\\^&|\rX|a", false, true},
		{"component gap", "H|\\^&|\rX||a^b^c", false, false},
		{"non-struct segment", "H|\\^&|\rS|a", false, true},
		{"recursive trigger", "H|\\^&|REC", true, true},
	}
	for _, item := range list {
		t.Run(item.Name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panic: %v", r)
				}
			}()
			dec := NewDecoder(shapeRegistry{}, nil)
			var err error
			if item.Group {
				_, err = dec.Decode([]byte(item.Input))
			} else {
				_, err = dec.DecodeList([]byte(item.Input))
			}
			if item.Err != (err != nil) {
				t.Fatalf("err=%v, want error=%t", err, item.Err)
			}
		})
	}

	t.Run("encode nested component", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("panic: %v", r)
			}
		}()
		_, err := NewEncoder(nil).Encode(shapeX{N: shapeNested{In: shapeInner{A: "a"}}})
		if err == nil {
			t.Fatal("expected error")
		}
	})
}
