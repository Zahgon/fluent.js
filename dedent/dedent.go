// Ported from fluent.js (Apache-2.0), fluent-dedent/src/index.ts.

// Package dedent strips the indentation of embedded Fluent code: a port of @fluent/dedent.
package dedent

import (
	"fmt"
	"strings"

	"github.com/projectfluent/fluent-go"
)

// isBlank matches /^[ \t]*$/: a blank line may contain spaces and tabs.
func isBlank(line string) bool {
	return strings.Trim(line, " \t") == ""
}

func rangeError(message string) *fluent.Error {
	return &fluent.Error{Name: "RangeError", Message: message}
}

// Ftl strips the indent of the last line from each line of code, and removes the first
// and the last line. The snippet must start on a new line and end on a line of its own;
// otherwise, or if a line is not indented like the last one, it raises RangeError.
//
// code is the template literal's cooked text, with any values already substituted.
func Ftl(code string) string {
	lines := strings.Split(code, "\n")

	if !isBlank(lines[0]) {
		panic(rangeError("Content must start on a new line."))
	}
	lines = lines[1:]

	if len(lines) == 0 || !isBlank(lines[len(lines)-1]) {
		panic(rangeError("Closing delimiter must appear on a new line."))
	}
	commonIndent := lines[len(lines)-1]
	lines = lines[:len(lines)-1]

	dedented := make([]string, len(lines))
	for i, line := range lines {
		// Empty lines are preserved even if technically they are not indented at all.
		// An empty commonIndent keeps every indent.
		if line == "" || commonIndent == "" {
			dedented[i] = line
			continue
		}

		// The indentation of the line must match commonIndent exactly.
		if !strings.HasPrefix(line, commonIndent) {
			panic(rangeError(fmt.Sprintf("Insufficient indentation in line %d.", i+1)))
		}
		dedented[i] = line[len(commonIndent):]
	}

	return strings.Join(dedented, "\n")
}
