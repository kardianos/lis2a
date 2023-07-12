package lis2a2

// Registry implements the required interface for unmarshalling data.
var Registry = registry{}

type registry struct{}

func (registry) Version() string {
	return Version
}
func (registry) ControlSegment() map[string]any {
	return ControlSegmentRegistry
}
func (registry) Segment() map[string]any {
	return SegmentRegistry
}
func (registry) Trigger() map[string]any {
	return TriggerRegistry
}
func (registry) DataType() map[string]any {
	return DataTypeRegistry
}

// Version of this LIS2A package.
var Version = `lis2a2`

// Segments specific to file and batch control.
var ControlSegmentRegistry = map[string]any{}

// Segment lookup by ID.
var SegmentRegistry = map[string]any{
	"H": H{},
	"P": P{},
	"O": O{},
	"R": R{},
	"C": C{},
	"Q": Q{},
	"S": S{},
	"M": M{},
	"L": L{},
}

// Trigger lookup by ID.
var TriggerRegistry = map[string]any{
	"NCCLS LIS2-A": LIS2_A2{},
	"E 1394-97":    LIS2_A2{},
	"E_1394-97":    LIS2_A2{},
}

// Data Type lookup by ID.
var DataTypeRegistry = map[string]any{
	"ST": *(new(ST)),
}
