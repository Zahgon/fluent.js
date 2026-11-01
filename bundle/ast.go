// Ported from fluent.js (Apache-2.0), fluent-bundle/src/ast.ts.

package bundle

// Pattern is a translation unit: pass it back to FluentBundle.FormatPattern. It is a
// string for a simple pattern, a *complexPattern otherwise, and nil is the original's
// null.
type Pattern = any

// complexPattern is a pattern with placeables. Its pointer is its identity for the
// detection of cycles.
type complexPattern struct {
	// elements are strings and expressions.
	elements []any
}

// Message is a raw message, or a term when its ID starts with a dash. Value is nil for
// a message with attributes only.
type Message struct {
	ID         string
	Value      Pattern
	Attributes map[string]Pattern
}

// expression is a parsed expression; resolver.go resolves it to a FluentValue.
type expression interface {
	resolve(scope *Scope) FluentValue
}

// stringLiteral is the original's {type: "str"}.
type stringLiteral struct{ value string }

// numberLiteral is the original's {type: "num"}.
type numberLiteral struct {
	value     float64
	precision int
}

// variableReference is the original's {type: "var"}.
type variableReference struct{ name string }

// messageReference is the original's {type: "mesg"}. An empty attr is null.
type messageReference struct{ name, attr string }

// termReference is the original's {type: "term"}. An empty attr is null.
type termReference struct {
	name, attr string
	args       []argument
}

// functionReference is the original's {type: "func"}.
type functionReference struct {
	name string
	args []argument
}

// selectExpression is the original's {type: "select"}. An arrow followed by no
// variants parses, and then variants is nil: the original's object has neither
// variants nor star, and fails when it resolves.
type selectExpression struct {
	selector expression
	variants []variant
	star     int
}

// variant is one branch of a select expression.
type variant struct {
	key   expression
	value Pattern
}

// argument is one argument of a call; an empty name means it is positional.
type argument struct {
	name  string
	value expression
}
