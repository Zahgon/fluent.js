package intl

import (
	"math"
	"strconv"
	"strings"

	fluent "github.com/projectfluent/fluent-go"
)

func raise(name, message string) {
	panic(&fluent.Error{Name: name, Message: message})
}

func rangeError(message string) {
	raise("RangeError", message)
}

func typeError(message string) {
	raise("TypeError", message)
}

// jsToString is the ECMAScript ToString of an option value.
func jsToString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return numberToString(x)
	case bool:
		if x {
			return "true"
		}
		return "false"
	}
	return ""
}

// jsToNumber is the ECMAScript ToNumber of an option value.
func jsToNumber(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case bool:
		if x {
			return 1
		}
		return 0
	case string:
		s := strings.TrimFunc(x, isJSSpace)
		if s == "" {
			return 0
		}
		if len(s) > 2 && s[0] == '0' {
			base := 0
			switch s[1] {
			case 'x', 'X':
				base = 16
			case 'o', 'O':
				base = 8
			case 'b', 'B':
				base = 2
			}
			if base != 0 {
				n, err := strconv.ParseUint(s[2:], base, 64)
				if err != nil {
					return math.NaN()
				}
				return float64(n)
			}
		}
		if s == "Infinity" || s == "+Infinity" {
			return math.Inf(1)
		}
		if s == "-Infinity" {
			return math.Inf(-1)
		}
		for _, c := range s {
			if !(c >= '0' && c <= '9' || c == '.' || c == 'e' || c == 'E' || c == '+' || c == '-') {
				return math.NaN()
			}
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			if ne, ok := err.(*strconv.NumError); ok && ne.Err == strconv.ErrRange {
				return f
			}
			return math.NaN()
		}
		return f
	}
	return math.NaN()
}

func isJSSpace(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\r', '\v', '\f', 0x00a0, 0xfeff, 0x2028, 0x2029, 0x1680, 0x202f, 0x205f, 0x3000:
		return true
	}
	return r >= 0x2000 && r <= 0x200a
}

// jsToBoolean is the ECMAScript ToBoolean of an option value.
func jsToBoolean(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return x != ""
	case float64:
		return x != 0 && !math.IsNaN(x)
	}
	return false
}

// stringOption is GetOption with type string and an optional list of allowed values; "" means absent.
func stringOption(service, property string, v any, allowed ...string) string {
	if v == nil {
		return ""
	}
	s := jsToString(v)
	if len(allowed) > 0 {
		for _, a := range allowed {
			if a == s {
				return s
			}
		}
		rangeError("Value " + s + " out of range for " + service + " options property " + property)
	}
	return s
}

// numberOption is DefaultNumberOption over GetOption: absent gives fallback, reported through has.
func numberOption(property string, v any, min, max int, fallback int) (int, bool) {
	if v == nil {
		return fallback, false
	}
	n := jsToNumber(v)
	if math.IsNaN(n) || n < float64(min) || n > float64(max) {
		rangeError(property + " value is out of range.")
	}
	return int(math.Floor(n)), true
}

// isASCIIAlpha reports whether s is entirely ASCII letters.
func isASCIIAlpha(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
			return false
		}
	}
	return true
}

func isASCIIAlnum(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

func isASCIIDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func toTitle(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + strings.ToLower(s[1:])
}

// numberToString is the ECMAScript Number::toString with radix 10.
func numberToString(x float64) string {
	switch {
	case math.IsNaN(x):
		return "NaN"
	case math.IsInf(x, 1):
		return "Infinity"
	case math.IsInf(x, -1):
		return "-Infinity"
	case x == 0:
		return "0"
	}
	s := strconv.FormatFloat(math.Abs(x), 'e', -1, 64)
	mant, expText, _ := strings.Cut(s, "e")
	digits := strings.Replace(mant, ".", "", 1)
	e, _ := strconv.Atoi(expText)
	n := e + 1
	k := len(digits)
	var out string
	switch {
	case k <= n && n <= 21:
		out = digits + strings.Repeat("0", n-k)
	case 0 < n && n <= 21:
		out = digits[:n] + "." + digits[n:]
	case -6 < n && n <= 0:
		out = "0." + strings.Repeat("0", -n) + digits
	default:
		exp := strconv.Itoa(n - 1)
		if n-1 > 0 {
			exp = "+" + exp
		}
		if k == 1 {
			out = digits + "e" + exp
		} else {
			out = digits[:1] + "." + digits[1:] + "e" + exp
		}
	}
	if x < 0 {
		return "-" + out
	}
	return out
}
