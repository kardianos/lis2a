package lis2a

import (
	"fmt"
	"reflect"
)

type messageStructure interface {
	MessageStructureID() string
}

func newWalker(list []any, registry Registry, lenient bool) (*walker, error) {
	if len(list) == 0 {
		return nil, fmt.Errorf("list is empty")
	}

	root := list[0]
	ms, ok := root.(messageStructure)
	if !ok {
		return nil, fmt.Errorf("first segment must implement MessageStructureID, %T does not", root)
	}
	code := ms.MessageStructureID()
	if len(code) == 0 {
		return nil, fmt.Errorf("message structure code empty in %T", root)
	}
	tr := registry.Trigger()
	vex, ok := tr[code]
	if !ok {
		return nil, fmt.Errorf("message structure code not found %q", code)
	}
	tp := reflect.TypeOf(vex)
	if tp == nil {
		return nil, fmt.Errorf("message structure %q registered with a nil trigger", code)
	}

	// Map a linear structure onto a hierarchical structure.
	//
	// 1. Go through trigger struct, depth first.
	// 2. Create a tree, noting the single, optional, and repeated fields.
	// 3. Loop A: Starting at the last match, go forward to match.
	// 4. If no match from current location, go backwards to find match.
	// 5. If nothing matches, return error.
	// 6. Determine if array item(s) needs to be added in parent tree.
	// 7. Add any parent items.
	// 8. Add segment.
	// 9. Go to next segment. End Loop A.
	// 10. When all segments are processed, return trigger structure.

	w := &walker{
		triggerCode: code,
		registry:    registry,
		lenient:     lenient,
	}
	err := w.eat(nil, 0, tp, false)
	if err != nil {
		return nil, err
	}
	return w, nil
}

func (w *walker) process(list []any) (any, error) {
	for i, item := range list {
		err := w.digest(i+1, item)
		if err != nil {
			return nil, err
		}
	}

	if len(w.list) == 0 {
		return nil, fmt.Errorf("missing list item, this should not happen")
	}
	rootSI := w.list[0]
	if !rootSI.Set {
		return nil, fmt.Errorf("root value not set")
	}
	return rootSI.ActiveValue.Interface(), nil
}

// group a list of segments into hierarchical groups with a single root element.
func group(list []any, registry Registry, lenient bool) (any, error) {
	w, err := newWalker(list, registry, lenient)
	if err != nil {
		return nil, err
	}
	return w.process(list)
}

type linkType int

const (
	linkUnknown linkType = iota
	linkValue
	linkOpt
	linkList
)

func (lt linkType) String() string {
	switch lt {
	default:
		return ""
	case linkUnknown:
		return "unknown"
	case linkValue:
		return "value"
	case linkOpt:
		return "opt"
	case linkList:
		return "list"
	}
}

type structItem struct {
	Index       int // Field index within the parent struct.
	Parent      *structItem
	LinkType    linkType
	Type        reflect.Type
	ActiveValue reflect.Value
	Set         bool // A value has been assigned within the current parent instance.
	Leaf        bool // Is a leaf node, nothing beyond this node.
	InArray     bool // If this struct node is within an array (and thus can always be added to).
	Lead        int  // For group nodes, the field index of the member that must start the group; -1 if none.
}

// isAncestor reports if a is a proper ancestor of si.
func (si *structItem) isAncestor(a *structItem) bool {
	for p := si.Parent; p != nil; p = p.Parent {
		if p == a {
			return true
		}
	}
	return false
}

func (si *structItem) name() string {
	if si.Parent == nil {
		return si.Type.Name()
	}
	return si.Parent.Type.Field(si.Index).Name
}

type walker struct {
	triggerCode string // For error reporting.
	registry    Registry
	lenient     bool

	last int
	list []*structItem
}

func (w *walker) digest(line int, v any) error {
	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return fmt.Errorf("line %d: nil segment", line)
	}
	rt := rv.Type()
	if rt.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return fmt.Errorf("line %d: nil segment %T", line, v)
		}
		rt = rt.Elem()
	}
	// First look forward.
	for i := w.last; i < len(w.list); i++ {
		si := w.list[i]
		if !si.Leaf || si.Type != rt {
			continue
		}
		if w.full(si) {
			continue
		}
		return w.found(line, i, si, rv)
	}
	// If not found going forward, go backwards.
	for i := w.last - 1; i >= 0; i-- {
		si := w.list[i]
		if !si.Leaf || si.Type != rt {
			continue
		}
		if w.full(si) {
			continue
		}
		return w.found(line, i, si, rv)
	}

	gr := w.registry.ControlSegment()
	_, isControl := gr[rt.Name()]
	if isControl {
		// TODO: handle batch and control segments.
		return nil
	}
	return fmt.Errorf("line %d (%T) not expected at this position in trigger %q", line, v, w.triggerCode)
}

// found creates the parent tree and sets it up.
func (w *walker) found(line, index int, si *structItem, rv reflect.Value) error {
	w.last = index

	// Collect the nodes that need a new value, from the leaf upward.
	currentList := []*structItem{}
	current := si
	findList := si.Set // Current value is full, find the next list.
	hasList := false
	for current != nil {
		// Continue stepping up until either the active value is set or a slice is encountered.
		// If the current item is not set, always add.
		// If the current item is set, but full, continue.
		// A LinkList LinkType is never full.
		if !current.Set {
			if current.LinkType == linkList {
				hasList = true
			}
			currentList = append(currentList, current)
			current = current.Parent
			continue
		}
		// Break if no list is needed or if a list has already been found.
		if !findList || hasList {
			break
		}
		currentList = append(currentList, current)
		if current.LinkType == linkList {
			break
		}
		current = current.Parent
	}
	if len(currentList) == 0 {
		return fmt.Errorf("line %d: no position found for segment", line)
	}

	if !w.lenient {
		if err := w.checkStrict(line, currentList); err != nil {
			return err
		}
	}

	for i := len(currentList) - 1; i >= 0; i-- {
		c := currentList[i]
		if c.Parent == nil {
			// The root value just needs to be created when found.
			if c.Set {
				return fmt.Errorf("line %d: attempting to set a root value when already present", line)
			}
			c.ActiveValue = reflect.New(c.Type).Elem()
			c.Set = true
			w.resetBelow(c)
			continue
		}
		var set reflect.Value
		switch {
		case c.Leaf:
			set = rv
		default:
			set = reflect.New(c.Type)
		}
		pv := c.Parent.ActiveValue.Field(c.Index)
		switch c.LinkType {
		default:
			return fmt.Errorf("line %d: unknown link type %v", line, c.LinkType)
		case linkValue:
			if set.Kind() == reflect.Pointer {
				set = set.Elem()
			}
			if c.Set {
				return fmt.Errorf("line %d: expected empty value %s, value present", line, pv.Type())
			}
			pv.Set(set)
			c.ActiveValue = pv

		case linkOpt:
			if set.Kind() != reflect.Pointer {
				p := reflect.New(set.Type())
				p.Elem().Set(set)
				set = p
			}
			if c.Set {
				return fmt.Errorf("line %d: expected empty pointer %s, pointer is present", line, pv.Type())
			}
			pv.Set(set)
			c.ActiveValue = pv.Elem()

		case linkList:
			if set.Kind() == reflect.Pointer {
				set = set.Elem()
			}
			pv.Set(reflect.Append(pv, set))
			c.ActiveValue = pv.Index(pv.Len() - 1)
		}
		c.Set = true
		w.resetBelow(c)
	}

	return nil
}

// checkStrict verifies the segment is in order and that any group created
// for it is started by the group's lead segment.
//
// currentList is ordered from the leaf upward; the last entry is either the
// root or the topmost node that receives a new value.
func (w *walker) checkStrict(line int, currentList []*structItem) error {
	leaf := currentList[0]

	// Each newly created group must be created by its lead member.
	for i := 1; i < len(currentList); i++ {
		g := currentList[i]
		below := currentList[i-1]
		if g.Leaf {
			continue
		}
		if g.Lead != below.Index {
			return fmt.Errorf("line %d: %s segment found without the segment that starts %s", line, leaf.Type.Name(), g.Type.Name())
		}
	}

	// The segment must not come after a later sibling at any level.
	// Nodes below the topmost entry are in a new parent instance, so start at the top.
	for n := currentList[len(currentList)-1]; n != nil && n.Parent != nil; n = n.Parent {
		for _, sib := range w.list {
			if sib.Parent != n.Parent || sib.Index <= n.Index {
				continue
			}
			if sib.Set {
				return fmt.Errorf("line %d: %s segment found after %s", line, leaf.Type.Name(), sib.name())
			}
		}
	}
	return nil
}

// resetBelow clears the state of every node beneath si, as si now refers to a new value.
func (w *walker) resetBelow(si *structItem) {
	for _, item := range w.list {
		if item.isAncestor(si) {
			item.ActiveValue = reflect.Value{}
			item.Set = false
		}
	}
}

// full reports true if there is no place to put another value by setting an existing value
// or by adding an item (either self or parent).
func (w *walker) full(si *structItem) bool {
	if si.InArray {
		return false
	}
	return si.Set
}

func (w *walker) eat(parent *structItem, fieldIndex int, rt reflect.Type, inArray bool) error {
	var baseType reflect.Type
	var link linkType
	switch rt.Kind() {
	default:
		return fmt.Errorf("unknown kind: %v", rt.Kind())
	case reflect.Struct:
		link = linkValue
		baseType = rt
	case reflect.Slice:
		link = linkList
		baseType = rt.Elem()
		inArray = true
	case reflect.Pointer:
		link = linkOpt
		baseType = rt.Elem()
	}
	if baseType.Kind() != reflect.Struct {
		return fmt.Errorf("trigger member %v must be a struct, slice of struct, or pointer to struct", rt)
	}
	var currentTag tag
	if metaField, ok := baseType.FieldByName(lis2aMetaName); ok {
		var err error
		currentTag, err = parseTag(metaField.Name, metaField.Tag.Get(tagName))
		if err != nil {
			return err
		}
	}
	leaf := parent != nil
	switch currentTag.Type {
	case structTriggerGroup:
		leaf = false
	}
	if !leaf {
		for p := parent; p != nil; p = p.Parent {
			if p.Type == baseType {
				return fmt.Errorf("trigger type %v is recursive", baseType)
			}
		}
	}

	item := &structItem{
		Index:    fieldIndex,
		Parent:   parent,
		LinkType: link,
		Type:     baseType,
		Leaf:     leaf,
		InArray:  inArray,
		Lead:     -1,
	}
	w.list = append(w.list, item)

	if leaf {
		return nil
	}

	// Peek into linked struct type, get meta info.

	// For each field type we look at, be sure to look at the tag type.
	// Only look at "t" and "tg" types. The segments must be the leaf types.

	ct := baseType.NumField()
	for i := 0; i < ct; i++ {
		ft := baseType.Field(i)
		tag, err := parseTag(ft.Name, ft.Tag.Get(tagName))
		if err != nil {
			return err
		}
		if !tag.Present {
			continue
		}
		if tag.Meta {
			continue
		}
		if item.Lead < 0 {
			item.Lead = i
		}
		err = w.eat(item, i, ft.Type, inArray)
		if err != nil {
			return err
		}
	}

	return nil
}
