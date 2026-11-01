// Ported from fluent.js (Apache-2.0), fluent-bundle/src/types.ts.

package bundle

import (
	"fmt"
	"math"
	"reflect"

	"github.com/projectfluent/fluent-go/internal/intl"
	"github.com/projectfluent/fluent-go/internal/jsnum"
)

// NumberOptions are the options of Intl.NumberFormat: nil (absent), string, float64 or bool.
type NumberOptions = intl.NumberOptions

// DateTimeOptions are the options of Intl.DateTimeFormat: nil (absent), string, float64 or bool.
type DateTimeOptions = intl.DateTimeOptions

// FluentValue is a string or a FluentType.
type FluentValue = any

// FluentType is the base of Fluent's type system: a wrapped value that knows how to
// format itself.
type FluentType interface {
	// ValueOf unwraps the raw value.
	ValueOf() any
	// ToString formats the value. Implementations may use the scope for error reporting.
	ToString(scope *Scope) string
}

// FluentNone is a FluentType representing no correct value.
type FluentNone struct{ Value string }

// NewFluentNone returns a FluentNone with an optional fallback value, "???" by default.
func NewFluentNone(value ...string) *FluentNone {
	if len(value) > 0 {
		return &FluentNone{Value: value[0]}
	}
	return &FluentNone{Value: "???"}
}

// ValueOf returns the fallback value.
func (n *FluentNone) ValueOf() any { return n.Value }

// ToString returns the fallback value in braces.
func (n *FluentNone) ToString(scope *Scope) string { return "{" + n.Value + "}" }

// FluentNumber is a FluentType representing a number and its formatting options.
type FluentNumber struct {
	Value float64
	Opts  NumberOptions
}

// NewFluentNumber returns a FluentNumber with the given options.
func NewFluentNumber(value float64, opts NumberOptions) *FluentNumber {
	return &FluentNumber{Value: value, Opts: opts}
}

// ValueOf returns the number, a float64.
func (n *FluentNumber) ValueOf() any { return n.Value }

// ToString formats the number with Intl.NumberFormat. Without a scope, or after an
// error reported to the scope, it returns the number's JavaScript string.
func (n *FluentNumber) ToString(scope *Scope) string {
	if scope != nil {
		if formatted, ok := n.format(scope); ok {
			return formatted
		}
	}
	return jsnum.ToString(n.Value)
}

// format catches what the original's try block catches.
func (n *FluentNumber) format(scope *Scope) (formatted string, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			scope.ReportError(r)
		}
	}()
	return scope.numberFormat(n.Opts).Format(n.Value), true
}

// JSDate is a JavaScript Date: its time value in milliseconds, NaN when invalid.
type JSDate float64

// FluentDateTime is a FluentType representing a date and time and its formatting
// options. Value is a float64 or a JSDate, or whatever the ValueOf of a wrapped
// FluentType returned.
type FluentDateTime struct {
	Value any
	Opts  DateTimeOptions
}

// SupportsValue reports whether value can be a FluentDateTime: a number, a date, or a
// FluentType wrapping one.
func SupportsValue(value any) bool {
	switch v := value.(type) {
	case float64, JSDate:
		return true
	case FluentType:
		return SupportsValue(v.ValueOf())
	}
	return false
}

// NewFluentDateTime returns a FluentDateTime for a float64, a JSDate or a FluentType. A
// wrapped FluentType is unwrapped, but only a FluentDateTime passes on its options, which
// the given options override.
func NewFluentDateTime(value any, opts DateTimeOptions) *FluentDateTime {
	switch v := value.(type) {
	case *FluentDateTime:
		return &FluentDateTime{Value: v.Value, Opts: mergeDateTimeOptions(v.Opts, opts)}
	case FluentType:
		return &FluentDateTime{Value: v.ValueOf(), Opts: opts}
	}
	return &FluentDateTime{Value: value, Opts: opts}
}

// mergeDateTimeOptions is {...base, ...over} for options whose absence is nil.
func mergeDateTimeOptions(base, over DateTimeOptions) DateTimeOptions {
	merged, overrides := reflect.ValueOf(&base).Elem(), reflect.ValueOf(over)
	for i := range merged.NumField() {
		if option := overrides.Field(i); !option.IsNil() {
			merged.Field(i).Set(option)
		}
	}
	return base
}

// ValueOf returns the wrapped value.
func (d *FluentDateTime) ValueOf() any { return d.Value }

// ToNumber returns the time value in milliseconds. A value which is neither a number
// nor a date raises TypeError.
func (d *FluentDateTime) ToNumber() float64 {
	switch v := d.Value.(type) {
	case float64:
		return v
	case JSDate:
		return float64(v)
	case string:
		panic(typeError("Cannot use 'in' operator to search for 'epochMilliseconds' in " + v))
	}
	panic(typeError("Unwrapping a non-number value as a number"))
}

// ToString formats the date with Intl.DateTimeFormat. Without a scope, or after an
// error reported to the scope, it returns the date's ISO string, which raises RangeError
// for an invalid date.
func (d *FluentDateTime) ToString(scope *Scope) string {
	if scope != nil {
		if formatted, ok := d.format(scope); ok {
			return formatted
		}
	}
	switch v := d.Value.(type) {
	case float64:
		return jsnum.ToISOString(v)
	case JSDate:
		return jsnum.ToISOString(float64(v))
	case string:
		return v
	}
	return fmt.Sprint(d.Value)
}

// format catches what the original's try block catches.
func (d *FluentDateTime) format(scope *Scope) (formatted string, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			scope.ReportError(r)
		}
	}()
	return scope.dateTimeFormat(d.Opts).Format(d.timeValue()), true
}

// timeValue is ToNumber applied to the value, as Intl.DateTimeFormat.format does.
func (d *FluentDateTime) timeValue() float64 {
	switch v := d.Value.(type) {
	case float64:
		return v
	case JSDate:
		return float64(v)
	case string:
		return jsnum.ToNumber(v)
	}
	return math.NaN()
}

// undefinedValue is the type of Undefined.
type undefinedValue struct{}

// Undefined is the JavaScript undefined as the value of a variable, or as what a custom
// function returns. nil is the JavaScript null.
var Undefined any = undefinedValue{}

// typeOf is the typeof operator for a variable's value that is not a string, a number or
// a FluentType. Any other Go value stands for a JavaScript object.
func typeOf(value any) string {
	switch value.(type) {
	case nil:
		return "object"
	case bool:
		return "boolean"
	case undefinedValue:
		return "undefined"
	}
	if reflect.TypeOf(value).Kind() == reflect.Func {
		return "function"
	}
	return "object"
}

// objectPrototype holds the properties of Object.prototype that the name of a variable
// can spell: `name in args` finds them in a plain object. Their value is a function. The
// others (__proto__, __defineGetter__ and so on) start with an underscore, which no FTL
// identifier does, so a pattern can never ask for them.
var objectPrototype = map[string]bool{
	"constructor":          true,
	"hasOwnProperty":       true,
	"isPrototypeOf":        true,
	"propertyIsEnumerable": true,
	"toLocaleString":       true,
	"toString":             true,
	"valueOf":              true,
}

// inheritedFunction is the value of a variable found in objectPrototype.
func inheritedFunction() {}
