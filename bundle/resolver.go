// Ported from fluent.js (Apache-2.0), fluent-bundle/src/resolver.ts.

package bundle

import (
	"strconv"
	"strings"

	"github.com/projectfluent/fluent-go"
)

// maxPlaceables is how many placeables a single call to FormatPattern may expand. The
// limit protects against the Billion Laughs and Quadratic Blowup attacks.
const maxPlaceables = 100

// Unicode bidi isolation characters.
const (
	fsi = "⁨"
	pdi = "⁩"
)

func referenceError(message string) *fluent.Error {
	return &fluent.Error{Name: "ReferenceError", Message: message}
}

func rangeError(message string) *fluent.Error {
	return &fluent.Error{Name: "RangeError", Message: message}
}

// valueToString is value.toString(scope). A custom function may return anything, and
// what JavaScript would do with it is reproduced here: null and undefined have no
// toString, a number takes the scope for an invalid radix, and an object prints as one.
func valueToString(value FluentValue, scope *Scope) string {
	switch v := value.(type) {
	case string:
		return v
	case FluentType:
		return v.ToString(scope)
	case nil:
		panic(typeError("Cannot read properties of null (reading 'toString')"))
	case undefinedValue:
		panic(typeError("Cannot read properties of undefined (reading 'toString')"))
	case float64:
		panic(rangeError("toString() radix argument must be between 2 and 36"))
	case bool:
		return strconv.FormatBool(v)
	}
	return "[object Object]"
}

// matches compares a selector with a variant key. Only the plural category of a number
// selector can fail, and that exception is not caught here.
func matches(scope *Scope, selector, key FluentValue) bool {
	if keyText, ok := key.(string); ok {
		if selectorText, ok := selector.(string); ok {
			return keyText == selectorText
		}
		if number, ok := selector.(*FluentNumber); ok {
			return keyText == scope.pluralRules(number.Opts).Select(number.Value)
		}
		return false
	}

	keyNumber, keyIsNumber := key.(*FluentNumber)
	number, selectorIsNumber := selector.(*FluentNumber)
	return keyIsNumber && selectorIsNumber && keyNumber.Value == number.Value
}

// getArguments resolves the arguments of a call.
func getArguments(scope *Scope, args []argument) ([]FluentValue, map[string]FluentValue) {
	positional := []FluentValue{}
	named := map[string]FluentValue{}

	for _, arg := range args {
		if arg.name != "" {
			named[arg.name] = arg.value.resolve(scope)
		} else {
			positional = append(positional, arg.value.resolve(scope))
		}
	}
	return positional, named
}

func (e *stringLiteral) resolve(*Scope) FluentValue { return e.value }

func (e *numberLiteral) resolve(*Scope) FluentValue {
	return NewFluentNumber(e.value, NumberOptions{MinimumFractionDigits: float64(e.precision)})
}

func (e *variableReference) resolve(scope *Scope) FluentValue {
	none := NewFluentNone("$" + e.name)

	var arg any
	if scope.params != nil {
		// Inside a term reference it is fine to reference undefined parameters.
		param, ok := scope.params[e.name]
		if !ok {
			return none
		}
		arg = param
	} else if value, ok := scope.lookupArg(e.name); ok {
		arg = value
	} else {
		// In the top-level pattern or inside a message reference, missing variables
		// produce errors.
		scope.ReportError(referenceError("Unknown variable: $" + e.name))
		return none
	}

	switch v := arg.(type) {
	case FluentType:
		return v
	case string:
		return v
	case float64:
		return NewFluentNumber(v, NumberOptions{})
	case JSDate:
		return NewFluentDateTime(v, DateTimeOptions{})
	}

	scope.ReportError(typeError("Variable type not supported: $" + e.name + ", " + typeOf(arg)))
	return none
}

func (e *messageReference) resolve(scope *Scope) FluentValue {
	message := scope.bundle.messages[e.name]
	if message == nil {
		scope.ReportError(referenceError("Unknown message: " + e.name))
		return NewFluentNone(e.name)
	}

	if e.attr != "" {
		if attribute, ok := message.Attributes[e.attr]; ok {
			return resolvePattern(scope, attribute)
		}
		scope.ReportError(referenceError("Unknown attribute: " + e.attr))
		return NewFluentNone(e.name + "." + e.attr)
	}

	if message.Value != nil {
		return resolvePattern(scope, message.Value)
	}

	scope.ReportError(referenceError("No value: " + e.name))
	return NewFluentNone(e.name)
}

// resolve resolves a term with its own parameters. They do not nest: scope.params is
// null afterwards, not what it was before.
func (e *termReference) resolve(scope *Scope) FluentValue {
	id := "-" + e.name
	term := scope.bundle.terms[id]
	if term == nil {
		scope.ReportError(referenceError("Unknown term: " + id))
		return NewFluentNone(id)
	}

	pattern := term.Value
	if e.attr != "" {
		attribute, ok := term.Attributes[e.attr]
		if !ok {
			scope.ReportError(referenceError("Unknown attribute: " + e.attr))
			return NewFluentNone(id + "." + e.attr)
		}
		pattern = attribute
	}

	_, scope.params = getArguments(scope, e.args)
	resolved := resolvePattern(scope, pattern)
	scope.params = nil
	return resolved
}

func (e *functionReference) resolve(scope *Scope) (result FluentValue) {
	// Some functions are built in; others may be provided through the bundle's options.
	function := scope.bundle.functions[e.name]
	if function == nil {
		scope.ReportError(referenceError("Unknown function: " + e.name + "()"))
		return NewFluentNone(e.name + "()")
	}

	defer func() {
		if r := recover(); r != nil {
			scope.ReportError(r)
			result = NewFluentNone(e.name + "()")
		}
	}()
	positional, named := getArguments(scope, e.args)
	return function(positional, named)
}

func (e *selectExpression) resolve(scope *Scope) FluentValue {
	selector := e.selector.resolve(scope)
	if _, isNone := selector.(*FluentNone); isNone {
		return e.resolveDefault(scope)
	}

	if e.variants == nil {
		panic(typeError("variants is not iterable"))
	}

	// Match the selector against the key of each variant, in order.
	for _, variant := range e.variants {
		if matches(scope, selector, variant.key.resolve(scope)) {
			return resolvePattern(scope, variant.value)
		}
	}

	return e.resolveDefault(scope)
}

func (e *selectExpression) resolveDefault(scope *Scope) FluentValue {
	if e.variants == nil {
		panic(typeError("Cannot read properties of undefined (reading 'undefined')"))
	}
	return resolvePattern(scope, e.variants[e.star].value)
}

// resolveComplexPattern resolves anything that is not a simple pattern. Something
// other than a *complexPattern, such as the missing value of an attribute-only term,
// fails as it does in the original, on adding it to the WeakSet of dirty patterns.
func resolveComplexPattern(scope *Scope, pattern Pattern) FluentValue {
	ptn, ok := pattern.(*complexPattern)
	if !ok {
		panic(typeError("Invalid value used in weak set"))
	}

	if scope.dirty[ptn] {
		scope.ReportError(rangeError("Cyclic reference"))
		return NewFluentNone()
	}

	// Tag the pattern as dirty for the purpose of the current resolution.
	scope.dirty[ptn] = true
	var result strings.Builder

	// Wrap interpolations with isolation marks only in patterns of several elements.
	useIsolating := scope.bundle.useIsolating && len(ptn.elements) > 1

	for _, element := range ptn.elements {
		if text, isText := element.(string); isText {
			result.WriteString(scope.bundle.transform(text))
			continue
		}

		scope.placeables++
		if scope.placeables > maxPlaceables {
			delete(scope.dirty, ptn)
			// Instantly bail out on this pattern: the length check protects against
			// excessive memory usage, and raising against eating up the CPU when long
			// placeables are deeply nested.
			panic(rangeError("Too many placeables expanded: " + strconv.Itoa(scope.placeables) +
				", max allowed is " + strconv.Itoa(maxPlaceables)))
		}

		if useIsolating {
			result.WriteString(fsi)
		}

		result.WriteString(valueToString(element.(expression).resolve(scope), scope))

		if useIsolating {
			result.WriteString(pdi)
		}
	}

	delete(scope.dirty, ptn)
	return result.String()
}

// resolvePattern resolves a simple or a complex pattern.
func resolvePattern(scope *Scope, pattern Pattern) FluentValue {
	if text, ok := pattern.(string); ok {
		return scope.bundle.transform(text)
	}
	return resolveComplexPattern(scope, pattern)
}
