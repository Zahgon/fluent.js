package syntax

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/projectfluent/fluent-go"
)

func spansOff() ParserOptions {
	off := false
	return ParserOptions{WithSpans: &off}
}

// raised runs f and returns the *fluent.Error it panics with, or nil.
func raised(f func()) (err *fluent.Error) {
	defer func() {
		if r := recover(); r != nil {
			err = r.(*fluent.Error)
		}
	}()
	f()
	return nil
}

func checkError(t *testing.T, what string, err *fluent.Error, name, message string) {
	t.Helper()
	if err == nil || err.Name != name || err.Message != message {
		t.Errorf("%s raised %v, want %s: %s", what, err, name, message)
	}
}

func junkAnnotation(t *testing.T, source string) *Annotation {
	t.Helper()
	res := Parse(source, ParserOptions{})
	for _, entry := range res.Body {
		if junk, ok := entry.(*Junk); ok {
			return junk.Annotations[0]
		}
	}
	t.Fatalf("Parse(%q) has no junk: %s", source, Serialize(res, SerializerOptions{}))
	return nil
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		source  string
		code    string
		args    []any
		message string
	}{
		{"!", "E0002", []any{}, "Expected an entry start"},
		{"foo", "E0003", []any{"="}, `Expected token: "="`},
		{"foo = {", "E0028", []any{}, "Expected an inline expression"},
		{"#x", "E0003", []any{" "}, `Expected token: " "`},
		{"#", "E0003", []any{" "}, `Expected token: " "`},
		{"-1", "E0004", []any{"a-zA-Z"}, `Expected a character from range: "a-zA-Z"`},
		{"foo = { 1. }", "E0004", []any{"0-9"}, `Expected a character from range: "0-9"`},
		{"foo =", "E0005", []any{"foo"}, `Expected message "foo" to have a value or attributes`},
		{"-foo =", "E0006", []any{"foo"}, `Expected term "-foo" to have a value`},
		{"foo = { bar() }", "E0008", []any{}, "The callee has to be an upper-case identifier or a term"},
		{"foo = { F(1: 2) }", "E0009", []any{}, "The argument name has to be a simple identifier"},
		{"foo = { $x ->\n  [a] b\n}", "E0010", []any{}, "Expected one of the variants to be marked as default (*)"},
		{"foo = { $x ->\n}", "E0011", []any{}, `Expected at least one variant after "->"`},
		{"foo = a\n  .b =", "E0012", []any{}, "Expected value"},
		{"foo = { $x ->\n *[", "E0013", []any{}, "Expected variant key"},
		{"foo = { F(a: $x) }", "E0014", []any{}, "Expected literal"},
		{"foo = { $x ->\n *[a] b\n *[c] d\n}", "E0015", []any{}, "Only one variant can be marked as default (*)"},
		{"foo = { bar ->\n *[a] b\n}", "E0016", []any{}, "Message references cannot be used as selectors"},
		{"foo = { -bar ->\n *[a] b\n}", "E0017", []any{}, "Terms cannot be used as selectors"},
		{"foo = { a.b ->\n *[a] b\n}", "E0018", []any{}, "Attributes of messages cannot be used as selectors"},
		{"foo = { -a.b }", "E0019", []any{}, "Attributes of terms cannot be used as placeables"},
		{"foo = { \"a\nb\" }", "E0020", []any{}, "Unterminated string expression"},
		{"foo = { F(a: 1, 2) }", "E0021", []any{}, "Positional arguments must not follow named arguments"},
		{"foo = { F(a: 1, a: 2) }", "E0022", []any{}, "Named arguments must be unique"},
		{`foo = { "\q" }`, "E0025", []any{"q"}, `Unknown escape sequence: \q.`},
		{`foo = { "\u12" }`, "E0026", []any{`\u12"`}, `Invalid Unicode escape sequence: \u12".`},
		{"foo = }", "E0027", []any{}, "Unbalanced closing brace in TextElement."},
		{"foo = { { $x } ->\n *[a] b\n}", "E0029", []any{}, "Expected simple expression as selector"},
	}
	for _, test := range tests {
		annotation := junkAnnotation(t, test.source)
		if annotation.Code != test.code || annotation.Message != test.message || !reflect.DeepEqual(annotation.Arguments, test.args) {
			t.Errorf("Parse(%q) annotation = %s %v %q, want %s %v %q", test.source,
				annotation.Code, annotation.Arguments, annotation.Message, test.code, test.args, test.message)
		}
	}
}

func TestEscapeErrorsAtTheEndOfSource(t *testing.T) {
	annotation := junkAnnotation(t, `foo = { "\`)
	if !reflect.DeepEqual(annotation.Arguments, []any{nil}) || annotation.Message != `Unknown escape sequence: \undefined.` {
		t.Errorf("annotation = %v %q, want an undefined argument", annotation.Arguments, annotation.Message)
	}

	annotation = junkAnnotation(t, `foo = { "\u12`)
	if !reflect.DeepEqual(annotation.Arguments, []any{`\u12undefined`}) {
		t.Errorf("arguments = %v, want the text \\u12undefined", annotation.Arguments)
	}
}

func TestLoneSurrogateInAnEscapeError(t *testing.T) {
	annotation := junkAnnotation(t, "foo = { \"\\😀\" }")
	if want := "Unknown escape sequence: \\\uFFFD."; annotation.Message != want {
		t.Errorf("message = %q, want %q: half of the astral character, as UTF-8 output shows it", annotation.Message, want)
	}
}

func TestSpansCountUTF16Units(t *testing.T) {
	res := Parse("😀\nfoo = 😀é{ $x }\nbar = 1", ParserOptions{})
	junk := res.Body[0].(*Junk)
	if want := (Span{Start: 0, End: 3}); *junk.Span != want {
		t.Errorf("junk span = %+v, want %+v", *junk.Span, want)
	}

	message := res.Body[1].(*Message)
	if want := (Span{Start: 3, End: 18}); *message.Span != want {
		t.Errorf("message span = %+v, want %+v", *message.Span, want)
	}
	text := message.Value.Elements[0].(*TextElement)
	if want := (Span{Start: 9, End: 12}); *text.Span != want {
		t.Errorf("text span = %+v, want %+v", *text.Span, want)
	}
	if want := (Span{Start: 0, End: 26}); *res.Span != want {
		t.Errorf("resource span = %+v, want %+v", *res.Span, want)
	}
}

func TestSpansOff(t *testing.T) {
	res := Parse("# c\nfoo = a {\"b\"}\n!\n", spansOff())
	var spans []string
	visitor := &Visitor{Hooks: map[string]func(*Visitor, Node){
		"Span": func(_ *Visitor, n Node) {
			span := n.(*Span)
			spans = append(spans, fmt.Sprintf("%d-%d", span.Start, span.End))
		},
	}}
	visitor.Visit(res)
	if want := []string{"18-18"}; !reflect.DeepEqual(spans, want) {
		t.Errorf("spans = %v, want %v: only the annotation has one", spans, want)
	}
}

func TestCommentAttachment(t *testing.T) {
	res := Parse("# a\nfoo = 1\n\n# b\n\nbar = 2\n\n## c\nbaz = 3\n\n# d\n!\n", ParserOptions{})
	var kinds []string
	for _, entry := range res.Body {
		kinds = append(kinds, entry.NodeType())
	}
	want := []string{"Message", "Comment", "Message", "GroupComment", "Message", "Comment", "Junk"}
	if !reflect.DeepEqual(kinds, want) {
		t.Fatalf("entries = %v, want %v", kinds, want)
	}
	message := res.Body[0].(*Message)
	if message.Comment == nil || message.Comment.Content != "a" || message.Span.Start != 0 {
		t.Errorf("first message: comment %+v, span %+v, want comment a and a span from 0", message.Comment, message.Span)
	}
}

func TestBlankCommentLineWithCRLF(t *testing.T) {
	res := Parse("# a\r\n#\r\n# b\r\n", ParserOptions{})
	if len(res.Body) != 2 {
		t.Fatalf("got %d entries, want 2: the raw CR of `#\\r\\n` ends the comment", len(res.Body))
	}
	if got := res.Body[0].(*Comment).Content; got != "a" {
		t.Errorf("first comment = %q, want %q", got, "a")
	}

	res = Parse("# a\n#\n# b\n", ParserOptions{})
	if got := res.Body[0].(*Comment).Content; got != "a\n\nb" {
		t.Errorf("comment with LF = %q, want %q", got, "a\n\nb")
	}
}

func TestDedent(t *testing.T) {
	tests := []struct {
		source string
		want   string
	}{
		{"foo =\n    a\n  b\n      c", "  a\nb\n    c"},
		{"foo = a\n  b\n\n   c   \n", "a\nb\n\n c"},
		{"foo = a\r\n  b\r\n", "a\nb"},
		{"foo = a\rb \r", "a\rb"},
	}
	for _, test := range tests {
		res := Parse(test.source, spansOff())
		text := res.Body[0].(*Message).Value.Elements[0].(*TextElement)
		if text.Value != test.want {
			t.Errorf("Parse(%q) text = %q, want %q", test.source, text.Value, test.want)
		}
	}
}

func TestParseEntry(t *testing.T) {
	parser := NewFluentParser(ParserOptions{})

	entry := parser.ParseEntry("# skipped\n\nfoo = 1\nbar = 2\n")
	message, ok := entry.(*Message)
	if !ok || message.ID.Name != "foo" || message.Comment != nil {
		t.Errorf("ParseEntry = %+v, want the message foo without a comment", entry)
	}

	if entry := parser.ParseEntry("#bad\nfoo = 1\n"); entry.NodeType() != "Junk" {
		t.Errorf("ParseEntry of a malformed comment = %s, want Junk", entry.NodeType())
	}

	if entry := parser.ParseEntry("# only comments\n"); entry.NodeType() != "Junk" {
		t.Errorf("ParseEntry with no entry = %s, want Junk", entry.NodeType())
	}
}

func TestSerializeRoundTrip(t *testing.T) {
	source := `# Comment
foo = Foo { $x } { -term("b", a: 1) } { NUMBER(1.5, minimumFractionDigits: 2) }
    .attr = Attr

## Group
-term = { $x ->
        [a] A
       *[1] One
            line
    }

### Resource

bar =
    Two
    lines
baz = .start
`
	res := Parse(source, ParserOptions{})
	got := Serialize(res, SerializerOptions{})
	want := `# Comment
foo = Foo { $x } { -term("b", a: 1) } { NUMBER(1.5, minimumFractionDigits: 2) }
    .attr = Attr

## Group

-term =
    { $x ->
        [a] A
       *[1]
            One
            line
    }

### Resource

bar =
    Two
    lines
baz = .start
`
	if got != want {
		t.Errorf("Serialize = %q, want %q", got, want)
	}
}

func TestSerializeJunk(t *testing.T) {
	res := Parse("foo = 1\n!\n# c\n", ParserOptions{})
	if got, want := Serialize(res, SerializerOptions{}), "foo = 1\n\n# c\n\n"; got != want {
		t.Errorf("without junk = %q, want %q", got, want)
	}
	if got, want := Serialize(res, SerializerOptions{WithJunk: true}), "foo = 1\n!\n\n# c\n\n"; got != want {
		t.Errorf("with junk = %q, want %q", got, want)
	}
}

func TestSerializeUnknownNodes(t *testing.T) {
	serializer := NewFluentSerializer(SerializerOptions{})
	tests := []struct {
		what    string
		call    func()
		message string
	}{
		{"nil resource", func() { serializer.Serialize(nil) }, "Unknown resource type: undefined"},
		{"Span as entry", func() { serializer.SerializeEntry(&Span{}) }, "Unknown entry type: [object Object]"},
		{"Identifier as expression", func() { SerializeExpression(&Identifier{}) }, "Unknown expression type: [object Object]"},
		{"nil expression", func() { SerializeExpression(nil) }, "Unknown expression type: undefined"},
		{"StringLiteral as key", func() { SerializeVariantKey(&StringLiteral{}) }, "Unknown variant key type: [object Object]"},
	}
	for _, test := range tests {
		checkError(t, test.what, raised(test.call), "Error", test.message)
	}
}

func TestStringLiteralParse(t *testing.T) {
	tests := []struct{ value, want string }{
		{`plain é`, "plain é"},
		{`\\ \" \u0041`, `\ " A`},
		{`\U01F600`, "😀"},
		{`\uD83D`, "\uFFFD"},
		{`\U00DFFF`, "\uFFFD"},
		{`\u12`, `\u12`},
		{`\q`, `\q`},
		{`\\u0041`, `\u0041`},
	}
	for _, test := range tests {
		if got := (&StringLiteral{Value: test.value}).Parse(); got != test.want {
			t.Errorf("Parse(%q) = %q, want %q", test.value, got, test.want)
		}
	}

	literal := &StringLiteral{Value: `\UFFFFFF`}
	checkError(t, "Parse", raised(func() { literal.Parse() }), "RangeError", "Invalid code point 16777215")
}

func TestNumberLiteralParse(t *testing.T) {
	tests := []struct {
		value     string
		want      float64
		precision int
	}{
		{"1", 1, 0},
		{"-1.50", -1.5, 2},
		{"0.0", 0, 1},
		{".5", 0.5, 0},
		{"abc", math.NaN(), 0},
		{"1.😀", 1, 2},
	}
	for _, test := range tests {
		got, precision := (&NumberLiteral{Value: test.value}).Parse()
		if (got != test.want && !math.IsNaN(test.want)) || precision != test.precision {
			t.Errorf("Parse(%q) = %v, %d, want %v, %d", test.value, got, precision, test.want, test.precision)
		}
	}
}

func TestVisitorOrderFollowsTheOriginalsKeys(t *testing.T) {
	res := Parse("# c\nfoo = a\n  .b = 1", ParserOptions{})
	var trace []string
	hooks := map[string]func(*Visitor, Node){}
	for _, kind := range []string{"Resource", "Message", "Identifier", "Pattern", "TextElement", "Attribute", "Comment", "Span"} {
		hooks[kind] = func(v *Visitor, n Node) {
			trace = append(trace, kind)
			v.GenericVisit(n)
		}
	}
	(&Visitor{Hooks: hooks}).Visit(res)
	want := "Resource Message Identifier Span Pattern TextElement Span Span Attribute Identifier Span " +
		"Pattern TextElement Span Span Span Comment Span Span Span"
	if got := strings.Join(trace, " "); got != want {
		t.Errorf("trace = %s, want %s", got, want)
	}
}

func TestVisitAnUndefinedAnnotationArgument(t *testing.T) {
	res := Parse(`foo = { "\`, ParserOptions{})
	checkError(t, "Visitor.Visit", raised(func() { (&Visitor{}).Visit(res) }),
		"TypeError", "Cannot read properties of undefined (reading 'type')")
	checkError(t, "Transformer.Visit", raised(func() { (&Transformer{}).Visit(res) }),
		"TypeError", "Cannot read properties of undefined (reading 'type')")

	res = Parse(`foo = { "\q" }`, ParserOptions{})
	(&Visitor{}).Visit(res)
	(&Transformer{}).Visit(res)
}

func TestTransformer(t *testing.T) {
	res := Parse("# c\nfoo = a { $x } b\n  .attr = c\n", spansOff())
	transformer := &Transformer{Hooks: map[string]func(*Transformer, Node) Node{
		"Comment":   func(*Transformer, Node) Node { return nil },
		"Attribute": func(*Transformer, Node) Node { return nil },
		"VariableReference": func(*Transformer, Node) Node {
			return &StringLiteral{Value: "s"}
		},
		"TextElement": func(_ *Transformer, n Node) Node {
			text := n.(*TextElement)
			text.Value = strings.ToUpper(text.Value)
			return text
		},
	}}
	if got := transformer.Visit(res); got != Node(res) {
		t.Errorf("Visit = %v, want the same resource", got)
	}

	message := res.Body[0].(*Message)
	if message.Comment != nil {
		t.Errorf("comment = %+v, want nil", message.Comment)
	}
	if message.Attributes == nil || len(message.Attributes) != 0 {
		t.Errorf("attributes = %#v, want a rebuilt empty slice", message.Attributes)
	}
	if got, want := Serialize(res, SerializerOptions{}), "foo = A { \"s\" } B\n"; got != want {
		t.Errorf("Serialize = %q, want %q", got, want)
	}
}

func TestSerializeAfterRemoval(t *testing.T) {
	removal := func(kind string) *Transformer {
		return &Transformer{Hooks: map[string]func(*Transformer, Node) Node{
			kind: func(*Transformer, Node) Node { return nil },
		}}
	}
	tests := []struct {
		source  string
		kind    string
		name    string
		message string
	}{
		{"foo = bar", "Identifier", "TypeError", "Cannot read properties of undefined (reading 'name')"},
		{"-foo = bar", "Pattern", "TypeError", "Cannot read properties of undefined (reading 'elements')"},
		{"foo = { BAR() }", "CallArguments", "TypeError", "Cannot read properties of undefined (reading 'positional')"},
		{"foo = { $x }", "VariableReference", "Error", "Unknown expression type: undefined"},
		{"foo = { $x ->\n *[1] a\n}", "NumberLiteral", "Error", "Unknown variant key type: undefined"},
	}
	for _, test := range tests {
		res := Parse(test.source, ParserOptions{})
		removal(test.kind).Visit(res)
		checkError(t, test.source+" without "+test.kind,
			raised(func() { Serialize(res, SerializerOptions{}) }), test.name, test.message)
	}
}

func TestTransformerCannotStoreAnotherTypeInATypedField(t *testing.T) {
	res := Parse("foo = bar", ParserOptions{})
	transformer := &Transformer{Hooks: map[string]func(*Transformer, Node) Node{
		"Identifier": func(*Transformer, Node) Node { return &Comment{} },
	}}
	err := raised(func() { transformer.Visit(res) })
	if err == nil || err.Name != "TypeError" {
		t.Errorf("Visit raised %v, want a TypeError", err)
	}
}

func TestClone(t *testing.T) {
	res := Parse("# c\nfoo = a { -t(1, b: 2) }\n  .c = d\n!\n", ParserOptions{})
	clone := Clone(res).(*Resource)
	if !reflect.DeepEqual(res, clone) {
		t.Errorf("Clone differs from the original")
	}

	clone.Body[0].(*Message).ID.Name = "changed"
	clone.Body[0].(*Message).Span.Start = 99
	clone.Body[1].(*Junk).Annotations[0].Arguments = nil
	original := res.Body[0].(*Message)
	if original.ID.Name != "foo" || original.Span.Start != 0 {
		t.Errorf("changing the clone changed the original: %s, %d", original.ID.Name, original.Span.Start)
	}
}

func TestEquals(t *testing.T) {
	withSpans := Parse("foo = a { $x }", ParserOptions{})
	withoutSpans := Parse("foo = a { $x }", spansOff())
	if !Equals(withSpans, withoutSpans) {
		t.Errorf("Equals ignores spans, want true")
	}
	if EqualsIgnoring(withSpans, withoutSpans, nil) {
		t.Errorf("EqualsIgnoring with nil ignores nothing, want false")
	}
	if !EqualsIgnoring(withSpans, withSpans, nil) {
		t.Errorf("a resource must equal itself")
	}
	if Equals(withSpans, Parse("foo = a { $y }", ParserOptions{})) {
		t.Errorf("Equals of different variables, want false")
	}

	comment, group := &Comment{Content: "c"}, &GroupComment{Content: "c"}
	if Equals(comment, group) {
		t.Errorf("Equals of different types, want false")
	}
	if !EqualsIgnoring(comment, group, []string{"type"}) {
		t.Errorf("EqualsIgnoring with type ignored, want true")
	}
	if !EqualsIgnoring(&StringLiteral{Value: "1"}, &NumberLiteral{Value: "1"}, []string{"type"}) {
		t.Errorf("EqualsIgnoring across literals with type ignored, want true")
	}
	if !EqualsIgnoring(&Span{Start: 1, End: 2}, &Span{Start: 3, End: 2}, []string{"start"}) {
		t.Errorf("EqualsIgnoring with start ignored, want true")
	}
	if Equals(&Message{ID: &Identifier{Name: "a"}}, &Message{ID: &Identifier{Name: "a"}, Comment: &Comment{}}) {
		t.Errorf("Equals of a nil and a set comment, want false")
	}
}

func TestEqualsAnnotationArguments(t *testing.T) {
	a := &Annotation{Code: "E", Arguments: []any{"x", nil}}
	b := &Annotation{Code: "E", Arguments: []any{"x", nil}}
	if !Equals(a, b) {
		t.Errorf("Equals of the same arguments, want true")
	}
	b.Arguments = []any{"x", "y"}
	if Equals(a, b) {
		t.Errorf("Equals of different arguments, want false")
	}
	nan := []any{math.NaN()}
	if Equals(&Annotation{Arguments: nan}, &Annotation{Arguments: nan}) {
		t.Errorf("NaN is not === NaN, want false")
	}
}

func TestOffsets(t *testing.T) {
	tests := []struct {
		source string
		pos    int
		line   int
		column int
	}{
		{"", 0, 0, 0},
		{"a\nb", 0, 0, 0},
		{"a\nb", 1, 0, 1},
		{"a\nb", 2, 1, 0},
		{"a\nb", 3, 1, 1},
		{"a\nb", 99, 1, 97},
		{"a\nb", -1, 0, -1},
		{"\nfoo", 0, 0, -1},
		{"😀\n😀", 3, 1, 0},
		{"😀\n😀", 4, 1, 1},
		{"😀", 1, 0, 1},
	}
	for _, test := range tests {
		if got := LineOffset(test.source, test.pos); got != test.line {
			t.Errorf("LineOffset(%q, %d) = %d, want %d", test.source, test.pos, got, test.line)
		}
		if got := ColumnOffset(test.source, test.pos); got != test.column {
			t.Errorf("ColumnOffset(%q, %d) = %d, want %d", test.source, test.pos, got, test.column)
		}
	}
}
