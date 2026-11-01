package dedent

import (
	"testing"

	"github.com/projectfluent/fluent-go"
)

func TestFtl(t *testing.T) {
	tests := []struct{ code, want string }{
		{"\n  foo = bar\n  ", "foo = bar"},
		{" \t\n  foo\n\n    bar\n  ", "foo\n\n  bar"},
		{"\n\tfoo\n\t", "foo"},
		{"\n  foo\n", "  foo"},
		{"\n\n", ""},
		{"\n", ""},
		{"\n  é😀\n  ", "é😀"},
		{"\n  \n  ", ""},
	}
	for _, test := range tests {
		if got := Ftl(test.code); got != test.want {
			t.Errorf("Ftl(%q) = %q, want %q", test.code, got, test.want)
		}
	}
}

func TestFtlErrors(t *testing.T) {
	tests := []struct{ code, message string }{
		{"foo\n", "Content must start on a new line."},
		{"\r\n  foo\n  ", "Content must start on a new line."},
		{"", "Closing delimiter must appear on a new line."},
		{" ", "Closing delimiter must appear on a new line."},
		{"\n  foo", "Closing delimiter must appear on a new line."},
		{"\n  foo\n  x", "Closing delimiter must appear on a new line."},
		{"\n  foo\n bar\n  ", "Insufficient indentation in line 2."},
		{"\n foo\n  ", "Insufficient indentation in line 1."},
		{"\n \n  ", "Insufficient indentation in line 1."},
		{"\n\tfoo\n  ", "Insufficient indentation in line 1."},
	}
	for _, test := range tests {
		func() {
			defer func() {
				err, ok := recover().(*fluent.Error)
				if !ok || err.Name != "RangeError" || err.Message != test.message {
					t.Errorf("Ftl(%q) raised %v, want RangeError: %s", test.code, err, test.message)
				}
			}()
			Ftl(test.code)
		}()
	}
}
