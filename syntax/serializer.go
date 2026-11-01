// Ported from fluent.js (Apache-2.0), fluent-syntax/src/serializer.ts.

package syntax

import (
	"strings"

	"github.com/projectfluent/fluent-go"
)

// indentExceptFirstLine indents the lines after the first.
func indentExceptFirstLine(content string) string {
	return strings.ReplaceAll(content, "\n", "\n    ")
}

func includesNewLine(element PatternElement) bool {
	text, ok := element.(*TextElement)
	return ok && strings.Contains(text.Value, "\n")
}

func isSelectExpr(element PatternElement) bool {
	placeable, ok := element.(*Placeable)
	if !ok {
		return false
	}
	_, ok = placeable.Expression.(*SelectExpression)
	return ok
}

func shouldStartOnNewLine(pattern *Pattern) bool {
	isMultiline := false
	for _, element := range pattern.Elements {
		isMultiline = isMultiline || isSelectExpr(element) || includesNewLine(element)
	}
	if !isMultiline {
		return false
	}
	if first, ok := pattern.Elements[0].(*TextElement); ok && first.Value != "" {
		// These characters may not appear first on a new line.
		switch first.Value[0] {
		case '[', '.', '*':
			return false
		}
	}
	return true
}

// SerializerOptions configures a FluentSerializer. WithJunk keeps Junk entries.
type SerializerOptions struct{ WithJunk bool }

// FluentSerializer turns an AST back into FTL.
type FluentSerializer struct{ withJunk bool }

// NewFluentSerializer returns a serializer with the given options.
func NewFluentSerializer(opts SerializerOptions) *FluentSerializer {
	return &FluentSerializer{withJunk: opts.WithJunk}
}

// unknownType builds the Error the serializer raises for a node it cannot handle.
func unknownType(what string, node Node) *fluent.Error {
	rendered := "[object Object]"
	if node == nil {
		rendered = "undefined"
	}
	return &fluent.Error{Name: "Error", Message: "Unknown " + what + " type: " + rendered}
}

// Serialize returns the FTL of a resource. A nil resource raises Error.
func (s *FluentSerializer) Serialize(res *Resource) string {
	if res == nil {
		panic(unknownType("resource", nil))
	}

	hasEntries := false
	var out strings.Builder
	for _, entry := range res.Body {
		if _, isJunk := entry.(*Junk); !isJunk || s.withJunk {
			out.WriteString(serializeEntry(entry, hasEntries))
			hasEntries = true
		}
	}
	return out.String()
}

// SerializeEntry returns the FTL of one entry, as the first of a resource.
func (s *FluentSerializer) SerializeEntry(entry Entry) string {
	return serializeEntry(entry, false)
}

// serializeEntry returns the FTL of an entry; comments get a blank line before them
// unless they come first.
func serializeEntry(entry Entry, hasEntries bool) string {
	prefix := ""
	if hasEntries {
		prefix = "\n"
	}
	switch e := entry.(type) {
	case *Message:
		return serializeMessage(e)
	case *Term:
		return serializeTerm(e)
	case *Comment:
		return prefix + serializeComment(e.Content, "#") + "\n"
	case *GroupComment:
		return prefix + serializeComment(e.Content, "##") + "\n"
	case *ResourceComment:
		return prefix + serializeComment(e.Content, "###") + "\n"
	case *Junk:
		return e.Content
	}
	panic(unknownType("entry", entry))
}

func serializeComment(content, prefix string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if line == "" {
			lines[i] = prefix
		} else {
			lines[i] = prefix + " " + line
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

// identifierName reads id.name, raising the original's TypeError on a removed child.
func identifierName(id *Identifier) string {
	if id == nil {
		panic(undefinedRead("name"))
	}
	return id.Name
}

func serializeMessage(message *Message) string {
	var out strings.Builder

	if message.Comment != nil {
		out.WriteString(serializeComment(message.Comment.Content, "#"))
	}

	out.WriteString(identifierName(message.ID) + " =")

	if message.Value != nil {
		out.WriteString(serializePattern(message.Value))
	}

	for _, attribute := range message.Attributes {
		out.WriteString(serializeAttribute(attribute))
	}

	out.WriteString("\n")
	return out.String()
}

func serializeTerm(term *Term) string {
	var out strings.Builder

	if term.Comment != nil {
		out.WriteString(serializeComment(term.Comment.Content, "#"))
	}

	out.WriteString("-" + identifierName(term.ID) + " =")
	out.WriteString(serializePattern(term.Value))

	for _, attribute := range term.Attributes {
		out.WriteString(serializeAttribute(attribute))
	}

	out.WriteString("\n")
	return out.String()
}

func serializeAttribute(attribute *Attribute) string {
	value := indentExceptFirstLine(serializePattern(attribute.Value))
	return "\n    ." + identifierName(attribute.ID) + " =" + value
}

func serializePattern(pattern *Pattern) string {
	if pattern == nil {
		panic(undefinedRead("elements"))
	}

	var content strings.Builder
	for _, element := range pattern.Elements {
		content.WriteString(serializeElement(element))
	}

	if shouldStartOnNewLine(pattern) {
		return "\n    " + indentExceptFirstLine(content.String())
	}
	return " " + indentExceptFirstLine(content.String())
}

func serializeElement(element PatternElement) string {
	switch e := element.(type) {
	case *TextElement:
		return e.Value
	case *Placeable:
		return serializePlaceable(e)
	}
	panic(unknownType("element", element))
}

func serializePlaceable(placeable *Placeable) string {
	switch expr := placeable.Expression.(type) {
	case *Placeable:
		return "{" + serializePlaceable(expr) + "}"
	case *SelectExpression:
		// Control the whitespace around the opening and the closing brace.
		return "{ " + SerializeExpression(expr) + "}"
	}
	return "{ " + SerializeExpression(placeable.Expression) + " }"
}

// SerializeExpression returns the FTL of an expression. Anything that is not an
// expression raises Error.
func SerializeExpression(expr Expression) string {
	switch e := expr.(type) {
	case *StringLiteral:
		return `"` + e.Value + `"`
	case *NumberLiteral:
		return e.Value
	case *VariableReference:
		return "$" + identifierName(e.ID)
	case *TermReference:
		out := "-" + identifierName(e.ID)
		if e.Attribute != nil {
			out += "." + e.Attribute.Name
		}
		if e.Arguments != nil {
			out += serializeCallArguments(e.Arguments)
		}
		return out
	case *MessageReference:
		out := identifierName(e.ID)
		if e.Attribute != nil {
			out += "." + e.Attribute.Name
		}
		return out
	case *FunctionReference:
		return identifierName(e.ID) + serializeCallArguments(e.Arguments)
	case *SelectExpression:
		out := SerializeExpression(e.Selector) + " ->"
		for _, variant := range e.Variants {
			out += serializeVariant(variant)
		}
		return out + "\n"
	case *Placeable:
		return serializePlaceable(e)
	}
	panic(unknownType("expression", expr))
}

func serializeVariant(variant *Variant) string {
	key := SerializeVariantKey(variant.Key)
	value := indentExceptFirstLine(serializePattern(variant.Value))

	if variant.Default {
		return "\n   *[" + key + "]" + value
	}
	return "\n    [" + key + "]" + value
}

func serializeCallArguments(args *CallArguments) string {
	if args == nil {
		panic(undefinedRead("positional"))
	}

	positional := make([]string, len(args.Positional))
	for i, arg := range args.Positional {
		positional[i] = SerializeExpression(arg)
	}
	named := make([]string, len(args.Named))
	for i, arg := range args.Named {
		named[i] = serializeNamedArgument(arg)
	}

	joinedPositional := strings.Join(positional, ", ")
	joinedNamed := strings.Join(named, ", ")
	if len(positional) > 0 && len(named) > 0 {
		return "(" + joinedPositional + ", " + joinedNamed + ")"
	}
	if joinedPositional != "" {
		return "(" + joinedPositional + ")"
	}
	return "(" + joinedNamed + ")"
}

func serializeNamedArgument(arg *NamedArgument) string {
	value := SerializeExpression(arg.Value)
	return identifierName(arg.Name) + ": " + value
}

// SerializeVariantKey returns the FTL of a variant key. Anything that is not a
// variant key raises Error.
func SerializeVariantKey(key VariantKey) string {
	switch k := key.(type) {
	case *Identifier:
		return k.Name
	case *NumberLiteral:
		return k.Value
	}
	panic(unknownType("variant key", key))
}
