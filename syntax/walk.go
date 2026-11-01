// Ported from fluent.js (Apache-2.0), fluent-syntax/src/ast.ts and fluent-syntax/src/visitor.ts.

package syntax

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/projectfluent/fluent-go"
)

// typeError builds the TypeError the original raises when it dereferences undefined.
func typeError(message string) *fluent.Error {
	return &fluent.Error{Name: "TypeError", Message: message}
}

// undefinedRead builds the TypeError for reading a property of undefined.
func undefinedRead(property string) *fluent.Error {
	return typeError("Cannot read properties of undefined (reading '" + property + "')")
}

// nodeStruct returns the struct behind a node. The original's Object.keys raises
// TypeError on undefined.
func nodeStruct(node Node) reflect.Value {
	if node == nil {
		panic(typeError("Cannot convert undefined or null to object"))
	}
	return reflect.ValueOf(node).Elem()
}

// isSetNode reports whether a pointer or interface field holds a node.
func isSetNode(field reflect.Value) bool {
	kind := field.Kind()
	return (kind == reflect.Pointer || kind == reflect.Interface) && !field.IsNil()
}

// Visitor is the read-only AST walker. A hook named after a node type ("Pattern"
// is the original's visitPattern) replaces the walk of nodes of that type; call
// GenericVisit from a hook to descend.
type Visitor struct {
	Hooks map[string]func(v *Visitor, node Node)
}

// Visit calls the node's hook, or GenericVisit if it has none.
func (v *Visitor) Visit(node Node) {
	if node == nil {
		panic(undefinedRead("type"))
	}
	if hook := v.Hooks[node.NodeType()]; hook != nil {
		hook(v, node)
		return
	}
	v.GenericVisit(node)
}

// GenericVisit visits the node's child nodes in the original's property order,
// its span included. Annotation arguments which are not nodes are skipped, but an
// undefined one (nil) raises TypeError, as it does in the original.
func (v *Visitor) GenericVisit(node Node) {
	s := nodeStruct(node)
	for i := range s.NumField() {
		field := s.Field(i)
		if isSetNode(field) {
			v.Visit(field.Interface().(Node))
		} else if field.Kind() == reflect.Slice {
			for j := range field.Len() {
				element := field.Index(j).Interface()
				if child, ok := element.(Node); ok || element == nil {
					v.Visit(child)
				}
			}
		}
	}
}

// Transformer is the read-and-write AST walker. A hook named after a node type
// returns the replacement of the node, or nil to remove it; call GenericVisit from
// a hook to descend.
type Transformer struct {
	Hooks map[string]func(t *Transformer, node Node) Node
}

// Visit calls the node's hook, or GenericVisit if it has none, and returns the result.
func (t *Transformer) Visit(node Node) Node {
	if node == nil {
		panic(undefinedRead("type"))
	}
	if hook := t.Hooks[node.NodeType()]; hook != nil {
		return hook(t, node)
	}
	return t.GenericVisit(node)
}

// GenericVisit transforms the node's child nodes in the original's property order,
// its span included, and returns the node. A removed child leaves its field nil
// (the original deletes the key); slices are rebuilt without the removed elements.
//
// A field declared with a concrete node type can only hold that type: a hook that
// returns another type raises TypeError, where the original would store it.
func (t *Transformer) GenericVisit(node Node) Node {
	s := nodeStruct(node)
	for i := range s.NumField() {
		field := s.Field(i)
		if isSetNode(field) {
			replaceChild(field, t.Visit(field.Interface().(Node)))
		} else if field.Kind() == reflect.Slice {
			field.Set(t.visitSlice(field))
		}
	}
	return node
}

// visitSlice returns a new slice of the transformed elements of a node's slice field.
func (t *Transformer) visitSlice(field reflect.Value) reflect.Value {
	rebuilt := reflect.MakeSlice(field.Type(), 0, field.Len())
	for i := range field.Len() {
		element := field.Index(i).Interface()
		child, isNode := element.(Node)
		if !isNode && element != nil {
			rebuilt = reflect.Append(rebuilt, field.Index(i))
			continue
		}
		if replaced := t.Visit(child); replaced != nil {
			rebuilt = reflect.Append(rebuilt, assignable(replaced, field.Type().Elem()))
		}
	}
	return rebuilt
}

// replaceChild stores a transformed child in its field; nil clears the field.
func replaceChild(field reflect.Value, replaced Node) {
	if replaced == nil {
		field.SetZero()
		return
	}
	field.Set(assignable(replaced, field.Type()))
}

// assignable wraps a node for storing in a field of the given type.
func assignable(node Node, target reflect.Type) reflect.Value {
	value := reflect.ValueOf(node)
	if !value.Type().AssignableTo(target) {
		panic(typeError(fmt.Sprintf("Cannot store a %s in a field of type %s", node.NodeType(), target)))
	}
	return value
}

// Clone returns a deep copy of the node.
func Clone(n Node) Node {
	if n == nil {
		panic(undefinedRead("clone"))
	}
	src := nodeStruct(n)
	dst := reflect.New(src.Type())
	for i := range src.NumField() {
		dst.Elem().Field(i).Set(cloneValue(src.Field(i)))
	}
	return dst.Interface().(Node)
}

// cloneValue deep-copies a field or slice element: nodes and slices are copied,
// anything else is shared.
func cloneValue(value reflect.Value) reflect.Value {
	switch value.Kind() {
	case reflect.Pointer, reflect.Interface:
		if node, ok := value.Interface().(Node); ok && !value.IsNil() {
			return reflect.ValueOf(Clone(node))
		}
	case reflect.Slice:
		if value.IsNil() {
			return value
		}
		clone := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		for i := range value.Len() {
			clone.Index(i).Set(cloneValue(value.Index(i)))
		}
		return clone
	}
	return value
}

// Equals reports whether two nodes are equal, ignoring their spans.
func Equals(a, b Node) bool {
	return EqualsIgnoring(a, b, []string{"span"})
}

// EqualsIgnoring reports whether two nodes are equal, ignoring the properties with
// the given names (the original's names: "span", "type", "value", ...) at every
// depth. nil ignores nothing. A nil span is the original's absent key, so a node
// with a span differs from one without it unless "span" is ignored.
func EqualsIgnoring(a, b Node, ignored []string) bool {
	if a == nil {
		panic(undefinedRead("equals"))
	}
	if b == nil {
		panic(typeError("Cannot convert undefined or null to object"))
	}
	aProps, bProps := properties(a, ignored), properties(b, ignored)
	if len(aProps) != len(bProps) {
		return false
	}
	for name, aValue := range aProps {
		bValue, ok := bProps[name]
		if !ok || !propertiesEqual(aValue, bValue, ignored) {
			return false
		}
	}
	return true
}

// properties returns a node's properties that are not ignored, by name, as the
// values the original would hold: a child node or nil, an []any, or a scalar.
func properties(node Node, ignored []string) map[string]any {
	s := nodeStruct(node)
	props := map[string]any{"type": node.NodeType()}
	for i := range s.NumField() {
		field := s.Field(i)
		name := strings.ToLower(s.Type().Field(i).Name) // ID is the original's id, and so on.
		if name == "span" && field.IsNil() {
			continue
		}
		props[name] = propertyValue(field)
	}
	for _, name := range ignored {
		delete(props, name)
	}
	return props
}

// propertyValue returns a field's value with typed nils and typed slices normalized.
func propertyValue(field reflect.Value) any {
	switch field.Kind() {
	case reflect.Pointer, reflect.Interface:
		if field.IsNil() {
			return nil
		}
	case reflect.Slice:
		values := make([]any, field.Len())
		for i := range values {
			values[i] = propertyValue(field.Index(i))
		}
		return values
	}
	return field.Interface()
}

// propertiesEqual compares two property values: arrays element by element, nodes
// with EqualsIgnoring and anything else with ===.
func propertiesEqual(a, b any, ignored []string) bool {
	aItems, aIsArray := a.([]any)
	bItems, bIsArray := b.([]any)
	if aIsArray && bIsArray {
		if len(aItems) != len(bItems) {
			return false
		}
		for i := range aItems {
			if !scalarsEqual(aItems[i], bItems[i], ignored) {
				return false
			}
		}
		return true
	}
	if aIsArray || bIsArray {
		return false
	}
	return scalarsEqual(a, b, ignored)
}

// scalarsEqual compares two values which are not both arrays.
func scalarsEqual(a, b any, ignored []string) bool {
	aNode, aIsNode := a.(Node)
	bNode, bIsNode := b.(Node)
	if aIsNode && bIsNode {
		return EqualsIgnoring(aNode, bNode, ignored)
	}
	return !aIsNode && !bIsNode && a == b
}
