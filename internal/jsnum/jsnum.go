// Package jsnum holds the ECMAScript number and date algorithms the library relies
// on: Number::toString, parseFloat, ToNumber on strings and Date.prototype.toISOString.
package jsnum

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/projectfluent/fluent-go"
)

// IsSpace reports whether r is ECMAScript WhiteSpace or LineTerminator, which is
// what \s matches and what String.prototype.trim removes.
func IsSpace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', 0xa0, 0x1680, 0x2028, 0x2029, 0x202f, 0x205f, 0x3000, 0xfeff:
		return true
	}
	return 0x2000 <= r && r <= 0x200a
}

// ToString is Number::toString with radix 10.
func ToString(v float64) string {
	switch {
	case math.IsNaN(v):
		return "NaN"
	case v == 0:
		return "0"
	case v < 0:
		return "-" + ToString(-v)
	case math.IsInf(v, 1):
		return "Infinity"
	}

	// Shortest digits that round-trip, d.ddde±x.
	mantissa, exponent, _ := strings.Cut(strconv.FormatFloat(v, 'e', -1, 64), "e")
	digits := strings.Replace(mantissa, ".", "", 1)
	k := len(digits)
	x, _ := strconv.Atoi(exponent)
	n := x + 1 // The value is 0.digits × 10^n.

	switch {
	case k <= n && n <= 21:
		return digits + strings.Repeat("0", n-k)
	case 0 < n && n <= 21:
		return digits[:n] + "." + digits[n:]
	case -6 < n && n <= 0:
		return "0." + strings.Repeat("0", -n) + digits
	}

	sign := "+"
	if n-1 < 0 {
		sign = "-"
	}
	magnitude := strconv.Itoa(max(n-1, 1-n))
	if k == 1 {
		return digits + "e" + sign + magnitude
	}
	return digits[:1] + "." + digits[1:] + "e" + sign + magnitude
}

// ParseFloat is the global parseFloat: it parses the longest decimal prefix after
// the leading whitespace and returns NaN when there is none.
func ParseFloat(s string) float64 {
	s = strings.TrimLeftFunc(s, IsSpace)
	end := decimalPrefix(s)
	if end == 0 {
		return math.NaN()
	}
	return decimalValue(s[:end])
}

// ToNumber is ToNumber applied to a string: the whole trimmed text must be a numeric
// literal. The empty string is 0 and anything else is NaN.
func ToNumber(s string) float64 {
	s = strings.TrimFunc(s, IsSpace)
	if s == "" {
		return 0
	}
	if len(s) > 2 && s[0] == '0' {
		switch s[1] {
		case 'x', 'X':
			return radixValue(s[2:], 16)
		case 'o', 'O':
			return radixValue(s[2:], 8)
		case 'b', 'B':
			return radixValue(s[2:], 2)
		}
	}
	if decimalPrefix(s) != len(s) {
		return math.NaN()
	}
	return decimalValue(s)
}

// decimalPrefix returns the length of the longest StrDecimalLiteral prefix of s.
func decimalPrefix(s string) int {
	i := 0
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}
	if strings.HasPrefix(s[i:], "Infinity") {
		return i + len("Infinity")
	}

	intEnd := skipDigits(s, i)
	end := intEnd
	if end < len(s) && s[end] == '.' {
		fractionEnd := skipDigits(s, end+1)
		if intEnd > i || fractionEnd > end+1 {
			end = fractionEnd
		}
	}
	if end == intEnd && intEnd == i {
		return 0
	}

	if end < len(s) && (s[end] == 'e' || s[end] == 'E') {
		exponent := end + 1
		if exponent < len(s) && (s[exponent] == '+' || s[exponent] == '-') {
			exponent++
		}
		if exponentEnd := skipDigits(s, exponent); exponentEnd > exponent {
			end = exponentEnd
		}
	}
	return end
}

func skipDigits(s string, i int) int {
	for i < len(s) && '0' <= s[i] && s[i] <= '9' {
		i++
	}
	return i
}

// decimalValue converts a StrDecimalLiteral. Out-of-range values become ±Infinity or
// ±0, as they do in ECMAScript, so the range error of strconv is ignored.
func decimalValue(s string) float64 {
	if strings.HasSuffix(s, "Infinity") {
		if s[0] == '-' {
			return math.Inf(-1)
		}
		return math.Inf(1)
	}
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

// radixValue converts the digits of a 0x, 0o or 0b literal, rounding to nearest.
func radixValue(digits string, radix int) float64 {
	if strings.ContainsAny(digits, "+-_") {
		return math.NaN()
	}
	value, ok := new(big.Int).SetString(digits, radix)
	if !ok {
		return math.NaN()
	}
	v, _ := new(big.Float).SetInt(value).Float64()
	return v
}

// ToISOString is new Date(ms).toISOString(), which raises RangeError "Invalid time
// value" when ms is not a valid time value.
func ToISOString(ms float64) string {
	if math.IsNaN(ms) || math.Abs(ms) > 8.64e15 {
		panic(&fluent.Error{Name: "RangeError", Message: "Invalid time value"})
	}

	const msPerDay = 86400000
	t := int64(ms) // TimeClip truncates towards zero.
	days := floorDiv(t, msPerDay)
	msOfDay := t - days*msPerDay
	year, month, day := civilFromDays(days)

	yearText := fmt.Sprintf("%04d", year)
	if year < 0 {
		yearText = fmt.Sprintf("-%06d", -year)
	} else if year > 9999 {
		yearText = fmt.Sprintf("+%06d", year)
	}
	return fmt.Sprintf("%s-%02d-%02dT%02d:%02d:%02d.%03dZ", yearText, month, day,
		msOfDay/3600000, msOfDay/60000%60, msOfDay/1000%60, msOfDay%1000)
}

func floorDiv(a, b int64) int64 {
	q := a / b
	if a%b != 0 && a < 0 {
		q--
	}
	return q
}

// civilFromDays converts days since 1970-01-01 to a proleptic Gregorian date.
func civilFromDays(days int64) (year, month, day int64) {
	z := days + 719468
	era := floorDiv(z, 146097)
	dayOfEra := z - era*146097
	yearOfEra := (dayOfEra - dayOfEra/1460 + dayOfEra/36524 - dayOfEra/146096) / 365
	dayOfYear := dayOfEra - (365*yearOfEra + yearOfEra/4 - yearOfEra/100)
	shiftedMonth := (5*dayOfYear + 2) / 153
	day = dayOfYear - (153*shiftedMonth+2)/5 + 1
	if shiftedMonth < 10 {
		month = shiftedMonth + 3
	} else {
		month = shiftedMonth - 9
	}
	year = yearOfEra + era*400
	if month <= 2 {
		year++
	}
	return year, month, day
}
