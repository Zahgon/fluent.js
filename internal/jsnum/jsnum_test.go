package jsnum

import (
	"errors"
	"math"
	"testing"

	"github.com/projectfluent/fluent-go"
)

func TestToString(t *testing.T) {
	tests := []struct {
		value float64
		want  string
	}{
		{0, "0"},
		{math.Copysign(0, -1), "0"},
		{math.NaN(), "NaN"},
		{math.Inf(1), "Infinity"},
		{math.Inf(-1), "-Infinity"},
		{1, "1"},
		{-1.5, "-1.5"},
		{100, "100"},
		{0.30000000000000004, "0.30000000000000004"},
		{1e21, "1e+21"},
		{1e20, "100000000000000000000"},
		{123456789012345680000, "123456789012345680000"},
		{1.5e21, "1.5e+21"},
		{1e-6, "0.000001"},
		{1e-7, "1e-7"},
		{1.25e-7, "1.25e-7"},
		{5e-324, "5e-324"},
		{1.7976931348623157e308, "1.7976931348623157e+308"},
	}
	for _, test := range tests {
		if got := ToString(test.value); got != test.want {
			t.Errorf("ToString(%v) = %q, want %q", test.value, got, test.want)
		}
	}
}

func sameFloat(a, b float64) bool {
	return math.Float64bits(a) == math.Float64bits(b) || (math.IsNaN(a) && math.IsNaN(b))
}

func TestParseFloat(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"1", 1},
		{"-0", math.Copysign(0, -1)},
		{"  \t\u00a0\ufeff\u20031.5abc", 1.5},
		{"\u00851", math.NaN()},
		{".5", 0.5},
		{"5.", 5},
		{"+.25e1x", 2.5},
		{"1e", 1},
		{"1e+", 1},
		{"1e3", 1000},
		{".", math.NaN()},
		{"-", math.NaN()},
		{"e5", math.NaN()},
		{"", math.NaN()},
		{"Infinityx", math.Inf(1)},
		{"-Infinity", math.Inf(-1)},
		{"infinity", math.NaN()},
		{"0x10", 0},
		{"1_000", 1},
		{"2e308", math.Inf(1)},
		{"-1e-400", math.Copysign(0, -1)},
	}
	for _, test := range tests {
		if got := ParseFloat(test.input); !sameFloat(got, test.want) {
			t.Errorf("ParseFloat(%q) = %v, want %v", test.input, got, test.want)
		}
	}
}

func TestToNumber(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"", 0},
		{"  ", 0},
		{" 12 ", 12},
		{"12x", math.NaN()},
		{"0x10", 16},
		{"0X1f", 31},
		{"0o17", 15},
		{"0b101", 5},
		{"0x", math.NaN()},
		{"-0x10", math.NaN()},
		{"0x-1", math.NaN()},
		{"-Infinity", math.Inf(-1)},
		{"1e3", 1000},
	}
	for _, test := range tests {
		if got := ToNumber(test.input); !sameFloat(got, test.want) {
			t.Errorf("ToNumber(%q) = %v, want %v", test.input, got, test.want)
		}
	}
}

func TestToISOString(t *testing.T) {
	tests := []struct {
		ms   float64
		want string
	}{
		{0, "1970-01-01T00:00:00.000Z"},
		{-1, "1969-12-31T23:59:59.999Z"},
		{1.5, "1970-01-01T00:00:00.001Z"},
		{-0.5, "1970-01-01T00:00:00.000Z"},
		{951782400000, "2000-02-29T00:00:00.000Z"},
		{1e12, "2001-09-09T01:46:40.000Z"},
		{8.64e15, "+275760-09-13T00:00:00.000Z"},
		{-8.64e15, "-271821-04-20T00:00:00.000Z"},
		{253402300800000, "+010000-01-01T00:00:00.000Z"},
		{253402300799999, "9999-12-31T23:59:59.999Z"},
		{-62167219200000, "0000-01-01T00:00:00.000Z"},
		{-62198755200000, "-000001-01-01T00:00:00.000Z"},
	}
	for _, test := range tests {
		if got := ToISOString(test.ms); got != test.want {
			t.Errorf("ToISOString(%v) = %q, want %q", test.ms, got, test.want)
		}
	}
}

func TestToISOStringInvalid(t *testing.T) {
	for _, ms := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), 8.64e15 + 1, -8.64e15 - 1} {
		func() {
			defer func() {
				err, _ := recover().(error)
				var fluentErr *fluent.Error
				if !errors.As(err, &fluentErr) || fluentErr.Name != "RangeError" || fluentErr.Message != "Invalid time value" {
					t.Errorf("ToISOString(%v) raised %v, want RangeError: Invalid time value", ms, err)
				}
			}()
			ToISOString(ms)
		}()
	}
}

func TestIsSpace(t *testing.T) {
	for _, r := range []rune{'\t', '\n', '\v', '\f', '\r', ' ', 0xa0, 0x1680, 0x2000, 0x200a, 0x2028, 0x2029, 0x202f, 0x205f, 0x3000, 0xfeff} {
		if !IsSpace(r) {
			t.Errorf("IsSpace(%U) = false, want true", r)
		}
	}
	for _, r := range []rune{'x', 0x85, 0x180e, 0x200b, 0x2060} {
		if IsSpace(r) {
			t.Errorf("IsSpace(%U) = true, want false", r)
		}
	}
}
