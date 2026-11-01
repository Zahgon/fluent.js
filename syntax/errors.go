// Ported from fluent.js (Apache-2.0), fluent-syntax/src/errors.ts.

package syntax

// parseError is the original's ParseError; only getEntryOrJunk catches it.
type parseError struct {
	code    string
	args    []any
	message string
}

// newParseError builds the parse error for a code. A nil argument is the
// original's undefined, which its template strings render as "undefined".
func newParseError(code string, args ...any) *parseError {
	if args == nil {
		args = []any{}
	}
	return &parseError{code: code, args: args, message: errorMessage(code, args)}
}

// errorMessage returns the message of a code. E0001, E0007, E0023 and E0024 do not
// appear because the original's parser never raises them.
func errorMessage(code string, args []any) string {
	switch code {
	case "E0002":
		return "Expected an entry start"
	case "E0003":
		return `Expected token: "` + argument(args) + `"`
	case "E0004":
		return `Expected a character from range: "` + argument(args) + `"`
	case "E0005":
		return `Expected message "` + argument(args) + `" to have a value or attributes`
	case "E0006":
		return `Expected term "-` + argument(args) + `" to have a value`
	case "E0008":
		return "The callee has to be an upper-case identifier or a term"
	case "E0009":
		return "The argument name has to be a simple identifier"
	case "E0010":
		return "Expected one of the variants to be marked as default (*)"
	case "E0011":
		return `Expected at least one variant after "->"`
	case "E0012":
		return "Expected value"
	case "E0013":
		return "Expected variant key"
	case "E0014":
		return "Expected literal"
	case "E0015":
		return "Only one variant can be marked as default (*)"
	case "E0016":
		return "Message references cannot be used as selectors"
	case "E0017":
		return "Terms cannot be used as selectors"
	case "E0018":
		return "Attributes of messages cannot be used as selectors"
	case "E0019":
		return "Attributes of terms cannot be used as placeables"
	case "E0020":
		return "Unterminated string expression"
	case "E0021":
		return "Positional arguments must not follow named arguments"
	case "E0022":
		return "Named arguments must be unique"
	case "E0025":
		return `Unknown escape sequence: \` + argument(args) + "."
	case "E0026":
		return "Invalid Unicode escape sequence: " + argument(args) + "."
	case "E0027":
		return "Unbalanced closing brace in TextElement."
	case "E0028":
		return "Expected an inline expression"
	default: // E0029
		return "Expected simple expression as selector"
	}
}

// argument renders the first error argument as a template string would.
func argument(args []any) string {
	if text, ok := args[0].(string); ok {
		return text
	}
	return "undefined"
}
