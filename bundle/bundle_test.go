package bundle

import (
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/projectfluent/fluent-go"
)

func off() *bool {
	value := false
	return &value
}

func newBundle(t *testing.T, source string, opts Options) *FluentBundle {
	t.Helper()
	if opts.UseIsolating == nil {
		opts.UseIsolating = off()
	}
	b := NewFluentBundle([]string{"en-US"}, opts)
	if errs := b.AddResource(NewFluentResource(source), AddResourceOptions{}); len(errs) != 0 {
		t.Fatalf("AddResource(%q) = %v, want no errors", source, errs)
	}
	return b
}

// errorTexts formats collected errors as their Error strings.
func errorTexts(errs []error) []string {
	texts := []string{}
	for _, err := range errs {
		texts = append(texts, err.Error())
	}
	return texts
}

// raised runs f and returns the value it panics with, or nil.
func raised(f func()) (value any) {
	defer func() { value = recover() }()
	f()
	return nil
}

func checkFormat(t *testing.T, b *FluentBundle, id string, args map[string]any, want string, wantErrs ...string) {
	t.Helper()
	message := b.GetMessage(id)
	if message == nil {
		t.Fatalf("GetMessage(%q) = nil", id)
	}
	errs := []error{}
	got := b.FormatPattern(message.Value, args, &errs)
	if got != want {
		t.Errorf("format %s = %q, want %q", id, got, want)
	}
	if wantErrs == nil {
		wantErrs = []string{}
	}
	if texts := errorTexts(errs); !reflect.DeepEqual(texts, wantErrs) {
		t.Errorf("format %s errors = %q, want %q", id, texts, wantErrs)
	}
}

func TestSimplePatternAndAttributes(t *testing.T) {
	b := newBundle(t, "foo = Foo\n  .attr = Attr\nbar =\n  .only = x\n", Options{})
	message := b.GetMessage("foo")
	if message.ID != "foo" || message.Value != "Foo" {
		t.Errorf("message = %+v, want foo with the value Foo", message)
	}
	if got := message.Attributes["attr"]; got != "Attr" {
		t.Errorf("attribute = %v, want Attr", got)
	}
	if b.GetMessage("bar").Value != nil {
		t.Errorf("value of an attribute-only message = %v, want nil", b.GetMessage("bar").Value)
	}
	if b.GetMessage("missing") != nil || b.HasMessage("missing") || !b.HasMessage("foo") {
		t.Errorf("GetMessage and HasMessage disagree on presence")
	}
}

func TestResourceParserLineStarts(t *testing.T) {
	// U+2028 and U+2029 start lines for /^/m, but CR does not, due to the lookbehind.
	b := newBundle(t, "a = 1\u2028b = 2\u2029c = 3\rd = 4", Options{})
	for _, id := range []string{"a", "b", "c"} {
		if !b.HasMessage(id) {
			t.Errorf("HasMessage(%q) = false, want true", id)
		}
	}
	if b.HasMessage("d") {
		t.Errorf("HasMessage(d) = true, want false: a message cannot start after CR")
	}
	if got, want := b.GetMessage("a").Value, "1\u2028b = 2\u2029c = 3\rd = 4"; got != want {
		t.Errorf("value = %q, want %q", got, want)
	}
}

func TestResourceSkipsSyntaxErrors(t *testing.T) {
	b := newBundle(t, "good1 = a\nbad = { $x\ngood2 = b\nbad2 = c }\ngood3 = d\n", Options{})
	for _, id := range []string{"good1", "good2", "good3"} {
		if !b.HasMessage(id) {
			t.Errorf("HasMessage(%q) = false, want true", id)
		}
	}
	for _, id := range []string{"bad", "bad2"} {
		if b.HasMessage(id) {
			t.Errorf("HasMessage(%q) = true, want false", id)
		}
	}
}

func TestResourceEscapeAboveUnicodeAborts(t *testing.T) {
	value := raised(func() { NewFluentResource("a = 1\nb = {\"\\UFFFFFF\"}\n") })
	err, ok := value.(*fluent.Error)
	if !ok || err.Name != "RangeError" || err.Message != "Invalid code point 16777215" {
		t.Errorf("NewFluentResource raised %v, want RangeError: Invalid code point 16777215", value)
	}
}

func TestResourceEscapes(t *testing.T) {
	b := newBundle(t, `a = {"\u0041\U01F600\uD83D\\\"x"}`, Options{})
	checkFormat(t, b, "a", nil, "A😀\uFFFD\\\"x")
}

func TestDedent(t *testing.T) {
	b := newBundle(t, "a =\n    x\n  y\n\n   z  \nb = p\n  q\n", Options{})
	checkFormat(t, b, "a", nil, "  x\ny\n\n z")
	checkFormat(t, b, "b", nil, "p\nq")
}

func TestAddResourceOverrides(t *testing.T) {
	b := NewFluentBundle([]string{"en-US"}, Options{})
	first := NewFluentResource("a = 1\n-b = 2\n")
	second := NewFluentResource("a = 3\n-b = 4\n")
	if errs := b.AddResource(first, AddResourceOptions{}); len(errs) != 0 {
		t.Fatalf("first AddResource = %v, want none", errs)
	}

	want := []string{`Error: Attempt to override an existing message: "a"`, `Error: Attempt to override an existing term: "-b"`}
	if got := errorTexts(b.AddResource(second, AddResourceOptions{})); !reflect.DeepEqual(got, want) {
		t.Errorf("AddResource = %q, want %q", got, want)
	}
	if got := b.GetMessage("a").Value; got != "1" {
		t.Errorf("value after a refused override = %v, want 1", got)
	}

	if errs := b.AddResource(second, AddResourceOptions{AllowOverrides: true}); len(errs) != 0 {
		t.Errorf("AddResource with overrides = %v, want none", errs)
	}
	if got := b.GetMessage("a").Value; got != "3" {
		t.Errorf("value after an override = %v, want 3", got)
	}
}

func TestVariables(t *testing.T) {
	b := newBundle(t, "a = {$x}", Options{})
	err := func(kind string) string { return "TypeError: Variable type not supported: $x, " + kind }

	checkFormat(t, b, "a", map[string]any{"x": "s"}, "s")
	checkFormat(t, b, "a", map[string]any{"x": NewFluentNone("n")}, "{n}")
	checkFormat(t, b, "a", nil, "{$x}", "ReferenceError: Unknown variable: $x")
	checkFormat(t, b, "a", map[string]any{}, "{$x}", "ReferenceError: Unknown variable: $x")
	checkFormat(t, b, "a", map[string]any{"x": nil}, "{$x}", err("object"))
	checkFormat(t, b, "a", map[string]any{"x": Undefined}, "{$x}", err("undefined"))
	checkFormat(t, b, "a", map[string]any{"x": true}, "{$x}", err("boolean"))
	checkFormat(t, b, "a", map[string]any{"x": func() {}}, "{$x}", err("function"))
	checkFormat(t, b, "a", map[string]any{"x": struct{}{}}, "{$x}", err("object"))
	checkFormat(t, b, "a", map[string]any{"x": 5}, "{$x}", err("object"))
}

func TestObjectPrototype(t *testing.T) {
	for _, name := range []string{"constructor", "hasOwnProperty", "isPrototypeOf", "propertyIsEnumerable", "toLocaleString", "toString", "valueOf"} {
		b := newBundle(t, "a = {$"+name+"}", Options{})
		checkFormat(t, b, "a", map[string]any{}, "{$"+name+"}", "TypeError: Variable type not supported: $"+name+", function")
		checkFormat(t, b, "a", nil, "{$"+name+"}", "ReferenceError: Unknown variable: $"+name)
		checkFormat(t, b, "a", map[string]any{name: "own"}, "own")
	}
}

func TestUnderscoreNamesDoNotParse(t *testing.T) {
	b := NewFluentBundle([]string{"en-US"}, Options{})
	b.AddResource(NewFluentResource("a = {$__proto__}\n__proto__ = x\n"), AddResourceOptions{})
	if b.HasMessage("a") || b.HasMessage("__proto__") {
		t.Errorf("identifiers cannot start with an underscore, so no message is expected")
	}
}

func TestReferences(t *testing.T) {
	b := newBundle(t, `
a = {b} {b.attr}
b = B
  .attr = Attr
c = {missing} {b.missing} {only}
only =
  .attr = x
d = {-missing} {-t.missing}
-t = T
`, Options{})
	checkFormat(t, b, "a", nil, "B Attr")
	checkFormat(t, b, "c", nil, "{missing} {b.missing} {only}",
		"ReferenceError: Unknown message: missing", "ReferenceError: Unknown attribute: missing", "ReferenceError: No value: only")
	checkFormat(t, b, "d", nil, "{-missing} {-t.missing}",
		"ReferenceError: Unknown term: -missing", "ReferenceError: Unknown attribute: missing")
}

func TestTermParameters(t *testing.T) {
	b := newBundle(t, `
-plain = {$x}|{$y}
a = {-plain}
b = {-plain(x: "1")}
c = {-plain("positional", y: "2")}
-attr = v
  .a = {$x}
d = {-attr.a(x: "1")}
-outer = {-inner}{$x}
-inner = i
e = {-outer(x: "1")}
-viamessage = {m}
m = {$x}
f = {-viamessage(x: "p")}
`, Options{})
	args := map[string]any{"x": "A"}
	checkFormat(t, b, "a", args, "{$x}|{$y}")
	checkFormat(t, b, "b", args, "1|{$y}")
	checkFormat(t, b, "c", args, "{$x}|2")
	checkFormat(t, b, "d", args, "1")
	// A nested term reference leaves the parameters null, so $x falls back to the arguments.
	checkFormat(t, b, "e", args, "iA")
	checkFormat(t, b, "f", args, "p")
}

func TestTermWithoutValue(t *testing.T) {
	b := newBundle(t, "-t =\n  .a = x\nm = before {-t}\nn = {-t.a}\n", Options{})
	checkFormat(t, b, "m", nil, "{???}", "TypeError: Invalid value used in weak set")
	checkFormat(t, b, "n", nil, "x")
}

func TestNilPattern(t *testing.T) {
	b := NewFluentBundle([]string{"en-US"}, Options{})
	errs := []error{}
	if got := b.FormatPattern(nil, nil, &errs); got != "{???}" {
		t.Errorf("FormatPattern(nil) = %q, want {???}", got)
	}
	if want := []string{"TypeError: Invalid value used in weak set"}; !reflect.DeepEqual(errorTexts(errs), want) {
		t.Errorf("errors = %q, want %q", errorTexts(errs), want)
	}
}

func TestSelect(t *testing.T) {
	b := newBundle(t, `
a = { $x ->
    [one] One
    [two] Two
   *[other] Other
}
b = { 1.0 ->
    [1] Number
   *[2] Default
}
c = { $x ->
   *[a] A
   *[b] B
}
`, Options{})
	checkFormat(t, b, "a", map[string]any{"x": "two"}, "Two")
	checkFormat(t, b, "a", map[string]any{"x": "none"}, "Other")
	checkFormat(t, b, "a", nil, "Other", "ReferenceError: Unknown variable: $x")
	checkFormat(t, b, "a", map[string]any{"x": NewFluentNone("n")}, "Other")
	checkFormat(t, b, "b", nil, "Number")
	checkFormat(t, b, "c", map[string]any{"x": "z"}, "B")
}

func TestSelectWithoutVariants(t *testing.T) {
	b := newBundle(t, "a = x { $x -> } y\n", Options{})
	checkFormat(t, b, "a", map[string]any{"x": "s"}, "{???}", "TypeError: variants is not iterable")
	checkFormat(t, b, "a", nil, "{???}", "ReferenceError: Unknown variable: $x",
		"TypeError: Cannot read properties of undefined (reading 'undefined')")
}

func TestCycles(t *testing.T) {
	b := newBundle(t, "a = {a}\nb = x{c}\nc = y{b}\n", Options{})
	checkFormat(t, b, "a", nil, "{???}", "RangeError: Cyclic reference")
	checkFormat(t, b, "b", nil, "xy{???}", "RangeError: Cyclic reference")

	message := b.GetMessage("a")
	value := raised(func() { b.FormatPattern(message.Value, nil, nil) })
	if err, ok := value.(*fluent.Error); !ok || err.Name != "RangeError" || err.Message != "Cyclic reference" {
		t.Errorf("FormatPattern with nil errs raised %v, want RangeError: Cyclic reference", value)
	}
}

func TestPlaceablesLimit(t *testing.T) {
	limit := "a = " + strings.Repeat(`{"s"}`, 100) + "\n"
	over := "b = " + strings.Repeat(`{"s"}`, 101) + "\n"
	b := newBundle(t, limit+over, Options{})
	checkFormat(t, b, "a", nil, strings.Repeat("s", 100))
	checkFormat(t, b, "b", nil, "{???}", "RangeError: Too many placeables expanded: 101, max allowed is 100")
}

func TestPatternsStayDirtyAfterAnExceptionCaughtByAFunction(t *testing.T) {
	functions := map[string]FluentFunction{
		"F":   func([]FluentValue, map[string]FluentValue) FluentValue { return "f" },
		"NIL": func([]FluentValue, map[string]FluentValue) FluentValue { return nil },
	}
	b := newBundle(t, "a = {F(b)}{b}\nb = {NIL()}\n", Options{Functions: functions})
	// The exception passes through b up to F(), which catches it, so b looks like a cycle.
	checkFormat(t, b, "a", nil, "{F()}{???}",
		"TypeError: Cannot read properties of null (reading 'toString')",
		"RangeError: Cyclic reference")
}

func TestIsolating(t *testing.T) {
	b := NewFluentBundle([]string{"en-US"}, Options{})
	b.AddResource(NewFluentResource("a = x {$x}\nb = {$x}\nc = {$x}{$x}\n"), AddResourceOptions{})
	args := map[string]any{"x": "v"}
	errs := []error{}
	if got, want := b.FormatPattern(b.GetMessage("a").Value, args, &errs), "x \u2068v\u2069"; got != want {
		t.Errorf("format = %q, want %q", got, want)
	}
	if got, want := b.FormatPattern(b.GetMessage("b").Value, args, &errs), "v"; got != want {
		t.Errorf("single placeable = %q, want %q", got, want)
	}
	if got, want := b.FormatPattern(b.GetMessage("c").Value, args, &errs), "\u2068v\u2069\u2068v\u2069"; got != want {
		t.Errorf("two placeables = %q, want %q", got, want)
	}
}

func TestTransform(t *testing.T) {
	b := newBundle(t, "a = text {\"literal\"} {b}\nb = ref\nc = simple\nd = { \"x\" ->\n *[x] variant\n}", Options{Transform: strings.ToUpper})
	checkFormat(t, b, "a", nil, "TEXT literal REF")
	checkFormat(t, b, "c", nil, "SIMPLE")
	checkFormat(t, b, "d", nil, "VARIANT")
}

func TestPanickingTransformOnASimplePatternIsNotCaught(t *testing.T) {
	boom := &fluent.Error{Name: "TypeError", Message: "boom"}
	b := newBundle(t, "a = simple\nb = {\"x\"} complex", Options{Transform: func(string) string { panic(boom) }})
	errs := []error{}
	if value := raised(func() { b.FormatPattern(b.GetMessage("a").Value, nil, &errs) }); value != any(boom) {
		t.Errorf("simple pattern raised %v, want the transform's error", value)
	}
	checkFormat(t, b, "b", nil, "{???}", "TypeError: boom")
}

func TestFunctions(t *testing.T) {
	boom := &fluent.Error{Name: "TypeError", Message: "boom"}
	functions := map[string]FluentFunction{
		"RAISE": func([]FluentValue, map[string]FluentValue) FluentValue { panic(boom) },
		"NAMED": func(positional []FluentValue, named map[string]FluentValue) FluentValue {
			return valueToString(positional[0], nil) + named["b"].(string)
		},
		"NONE":   func([]FluentValue, map[string]FluentValue) FluentValue { return NewFluentNone("n") },
		"NIL":    func([]FluentValue, map[string]FluentValue) FluentValue { return nil },
		"UNDEF":  func([]FluentValue, map[string]FluentValue) FluentValue { return Undefined },
		"NUM":    func([]FluentValue, map[string]FluentValue) FluentValue { return 5.0 },
		"BOOL":   func([]FluentValue, map[string]FluentValue) FluentValue { return true },
		"OBJECT": func([]FluentValue, map[string]FluentValue) FluentValue { return struct{}{} },
		"NILFN":  nil,
	}
	b := newBundle(t, `
raise = x {RAISE()} y
named = {NAMED("a", b: "c", b: "d")}
none = {NONE()}
nil = {NIL()}
undef = {UNDEF()}
num = {NUM()}
bool = {BOOL()}
object = {OBJECT()}
nilfn = {NILFN()}
unknown = {OTHER()}
arg = {NAMED(RAISE(), b: "c")}
unresolved = {NAMED({other}, b: "c")}
`, Options{Functions: functions})

	checkFormat(t, b, "raise", nil, "x {RAISE()} y", "TypeError: boom")
	checkFormat(t, b, "named", nil, "ad")
	checkFormat(t, b, "none", nil, "{n}")
	checkFormat(t, b, "nil", nil, "{???}", "TypeError: Cannot read properties of null (reading 'toString')")
	checkFormat(t, b, "undef", nil, "{???}", "TypeError: Cannot read properties of undefined (reading 'toString')")
	checkFormat(t, b, "num", nil, "{???}", "RangeError: toString() radix argument must be between 2 and 36")
	checkFormat(t, b, "bool", nil, "true")
	checkFormat(t, b, "object", nil, "[object Object]")
	checkFormat(t, b, "nilfn", nil, "{NILFN()}", "ReferenceError: Unknown function: NILFN()")
	checkFormat(t, b, "unknown", nil, "{OTHER()}", "ReferenceError: Unknown function: OTHER()")
	checkFormat(t, b, "arg", nil, "{RAISE()}c", "TypeError: boom")
	checkFormat(t, b, "unresolved", nil, "{other}c", "ReferenceError: Unknown message: other")

	message := b.GetMessage("raise")
	if value := raised(func() { b.FormatPattern(message.Value, nil, nil) }); value != any(boom) {
		t.Errorf("FormatPattern with nil errs raised %v, want the function's error", value)
	}
}

func TestPanicThatIsNotAnExceptionPropagates(t *testing.T) {
	functions := map[string]FluentFunction{
		"F": func([]FluentValue, map[string]FluentValue) FluentValue { panic("not an exception") },
	}
	b := newBundle(t, "a = {F()}", Options{Functions: functions})
	errs := []error{}
	if value := raised(func() { b.FormatPattern(b.GetMessage("a").Value, nil, &errs) }); value != "not an exception" {
		t.Errorf("FormatPattern raised %v, want the original value", value)
	}
	if len(errs) != 0 {
		t.Errorf("errors = %v, want none", errs)
	}
}

func TestNumberAndDateTimeOptions(t *testing.T) {
	var got []FluentValue
	functions := map[string]FluentFunction{
		"CAPTURE": func(positional []FluentValue, _ map[string]FluentValue) FluentValue {
			got = positional
			return ""
		},
	}
	b := newBundle(t, `
a = {CAPTURE(NUMBER(1.50, minimumFractionDigits: 3, foo: "bar", useGrouping: "false"))}
b = {CAPTURE(NUMBER(NUMBER(1, maximumFractionDigits: 2), maximumFractionDigits: 1))}
c = {CAPTURE(DATETIME(2, month: "long", foo: "bar"))}
d = {CAPTURE(DATETIME(DATETIME(2, month: "long", year: "numeric"), month: "short"))}
e = {CAPTURE(NUMBER(DATETIME(2, month: "long"), minimumFractionDigits: 1))}
`, Options{Functions: functions})

	format := func(id string) FluentValue {
		errs := []error{}
		b.FormatPattern(b.GetMessage(id).Value, nil, &errs)
		if len(errs) != 0 {
			t.Errorf("format %s errors = %v, want none", id, errs)
		}
		return got[0]
	}

	number := format("a").(*FluentNumber)
	want := NumberOptions{MinimumFractionDigits: 3.0, UseGrouping: "false"}
	if number.Value != 1.5 || number.Opts != want {
		t.Errorf("a = %+v, want 1.5 with %+v", number, want)
	}

	number = format("b").(*FluentNumber)
	want = NumberOptions{MinimumFractionDigits: 0.0, MaximumFractionDigits: 1.0}
	if number.Opts != want {
		t.Errorf("b options = %+v, want %+v", number.Opts, want)
	}

	date := format("c").(*FluentDateTime)
	if date.Value != 2.0 || date.Opts != (DateTimeOptions{Month: "long"}) {
		t.Errorf("c = %+v, want 2 with month long only", date)
	}

	date = format("d").(*FluentDateTime)
	if date.Opts != (DateTimeOptions{Month: "short", Year: "numeric"}) {
		t.Errorf("d options = %+v, want month short and year numeric", date.Opts)
	}

	number = format("e").(*FluentNumber)
	if number.Value != 2 || number.Opts != (NumberOptions{MinimumFractionDigits: 1.0}) {
		t.Errorf("e = %+v, want 2 with only the named option", number)
	}
}

func TestBuiltinErrors(t *testing.T) {
	b := newBundle(t, `
n = {NUMBER("s")}
d = {DATETIME("s")}
nn = {NUMBER()}
none = {NUMBER($missing)}
dnone = {DATETIME($missing)}
`, Options{})
	checkFormat(t, b, "n", nil, "{NUMBER()}", "TypeError: Invalid argument to NUMBER")
	checkFormat(t, b, "d", nil, "{DATETIME()}", "TypeError: Invalid argument to DATETIME")
	checkFormat(t, b, "nn", nil, "{NUMBER()}", "TypeError: Invalid argument to NUMBER")
	checkFormat(t, b, "none", nil, "{NUMBER($missing)}", "ReferenceError: Unknown variable: $missing")
	checkFormat(t, b, "dnone", nil, "{DATETIME($missing)}", "ReferenceError: Unknown variable: $missing")
}

func TestFluentTypes(t *testing.T) {
	none := NewFluentNone()
	if none.ValueOf() != "???" || none.ToString(nil) != "{???}" {
		t.Errorf("default FluentNone = %v, %q, want ??? and {???}", none.ValueOf(), none.ToString(nil))
	}

	number := NewFluentNumber(1.5, NumberOptions{})
	if number.ValueOf() != 1.5 || number.ToString(nil) != "1.5" {
		t.Errorf("FluentNumber = %v, %q, want 1.5", number.ValueOf(), number.ToString(nil))
	}
	if got := NewFluentNumber(math.Copysign(0, -1), NumberOptions{}).ToString(nil); got != "0" {
		t.Errorf("FluentNumber(-0) = %q, want 0", got)
	}

	date := NewFluentDateTime(JSDate(0), DateTimeOptions{})
	if date.ToString(nil) != "1970-01-01T00:00:00.000Z" || date.ToNumber() != 0 {
		t.Errorf("FluentDateTime = %q, %v, want the epoch", date.ToString(nil), date.ToNumber())
	}
	if date.ValueOf() != JSDate(0) {
		t.Errorf("ValueOf = %v, want the JSDate", date.ValueOf())
	}

	value := raised(func() { NewFluentDateTime(JSDate(math.NaN()), DateTimeOptions{}).ToString(nil) })
	if err, ok := value.(*fluent.Error); !ok || err.Name != "RangeError" || err.Message != "Invalid time value" {
		t.Errorf("invalid date raised %v, want RangeError: Invalid time value", value)
	}
}

func TestFluentDateTimeConstructor(t *testing.T) {
	inner := NewFluentDateTime(5.0, DateTimeOptions{Month: "long", Year: "numeric"})
	merged := NewFluentDateTime(inner, DateTimeOptions{Month: "short"})
	if merged.Value != 5.0 || merged.Opts != (DateTimeOptions{Month: "short", Year: "numeric"}) {
		t.Errorf("merged = %+v, want 5 with month short and year numeric", merged)
	}

	fromNumber := NewFluentDateTime(NewFluentNumber(7, NumberOptions{MinimumFractionDigits: 1.0}), DateTimeOptions{})
	if fromNumber.Value != 7.0 || fromNumber.Opts != (DateTimeOptions{}) {
		t.Errorf("from a number = %+v, want 7 without options", fromNumber)
	}

	fromNone := NewFluentDateTime(NewFluentNone("x"), DateTimeOptions{})
	if fromNone.ToString(nil) != "x" {
		t.Errorf("from a none = %q, want x", fromNone.ToString(nil))
	}
	value := raised(func() { fromNone.ToNumber() })
	want := "Cannot use 'in' operator to search for 'epochMilliseconds' in x"
	if err, ok := value.(*fluent.Error); !ok || err.Name != "TypeError" || err.Message != want {
		t.Errorf("ToNumber raised %v, want TypeError: %s", value, want)
	}
}

func TestSupportsValue(t *testing.T) {
	tests := []struct {
		value any
		want  bool
	}{
		{1.0, true},
		{math.NaN(), true},
		{JSDate(0), true},
		{NewFluentNumber(1, NumberOptions{}), true},
		{NewFluentDateTime(1.0, DateTimeOptions{}), true},
		{NewFluentNone("x"), false},
		{"s", false},
		{nil, false},
		{true, false},
		{1, false},
	}
	for _, test := range tests {
		if got := SupportsValue(test.value); got != test.want {
			t.Errorf("SupportsValue(%#v) = %v, want %v", test.value, got, test.want)
		}
	}
}

// reporter is a custom type that reports to the scope when it is formatted.
type reporter struct{ err any }

func (r *reporter) ValueOf() any { return r.err }

func (r *reporter) ToString(scope *Scope) string {
	scope.ReportError(r.err)
	return "reported"
}

func TestScopeReportError(t *testing.T) {
	b := newBundle(t, "a = {$x}", Options{})
	exception := &fluent.Error{Name: "RangeError", Message: "r"}

	checkFormat(t, b, "a", map[string]any{"x": &reporter{err: exception}}, "reported", "RangeError: r")

	message := b.GetMessage("a")
	errs := []error{}
	args := map[string]any{"x": &reporter{err: "other"}}
	if value := raised(func() { b.FormatPattern(message.Value, args, &errs) }); value != "other" {
		t.Errorf("reporting a non-exception raised %v, want it re-raised", value)
	}

	args = map[string]any{"x": &reporter{err: exception}}
	if value := raised(func() { b.FormatPattern(message.Value, args, nil) }); value != any(exception) {
		t.Errorf("reporting with nil errs raised %v, want it re-raised", value)
	}
}

func TestLocalesAndOptionsDefaults(t *testing.T) {
	locales := []string{"en-US", "en"}
	b := NewFluentBundle(locales, Options{})
	if !reflect.DeepEqual(b.Locales(), locales) {
		t.Errorf("Locales = %v, want %v", b.Locales(), locales)
	}
	if !b.useIsolating {
		t.Errorf("useIsolating defaults to false, want true")
	}
	if len(b.functions) != 2 {
		t.Errorf("functions = %d, want NUMBER and DATETIME", len(b.functions))
	}
}
