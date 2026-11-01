// Ported from fluent.js (Apache-2.0), fluent-syntax/src/index.ts.

package syntax

import "unicode/utf16"

// Parse parses a whole document with a new parser.
func Parse(source string, opts ParserOptions) *Resource {
	return NewFluentParser(opts).Parse(source)
}

// Serialize returns the FTL of a resource with a new serializer.
func Serialize(res *Resource, opts SerializerOptions) string {
	return NewFluentSerializer(opts).Serialize(res)
}

// LineOffset returns the zero-based line of a position, in UTF-16 code units. A
// position outside the source is clamped to it.
func LineOffset(source string, pos int) int {
	units := utf16.Encode([]rune(source))
	line := 0
	for _, unit := range units[:max(0, min(pos, len(units)))] {
		if unit == '\n' {
			line++
		}
	}
	return line
}

// ColumnOffset returns the zero-based column of a position, in UTF-16 code units.
// It looks for the last LF at or before pos-1, and lastIndexOf clamps a negative
// index to 0: at position 0 of a source that starts with LF the result is -1.
func ColumnOffset(source string, pos int) int {
	units := utf16.Encode([]rune(source))
	for i := max(0, min(pos-1, len(units)-1)); i >= 0 && i < len(units); i-- {
		if units[i] == '\n' {
			return pos - i - 1
		}
	}
	return pos
}
