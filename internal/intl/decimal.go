package intl

import (
	"math"
	"strconv"
	"strings"
)

// decimal is an exact decimal quantity: digits × 10^exp, plus the display width ICU tracks. digits has no leading
// zeros and no trailing zeros; zero has no digits.
type decimal struct {
	neg, nan, inf bool
	digits        []byte // '0'..'9'
	exp           int    // the power of ten of the last digit
	minInt        int
	minFrac       int
	maxInt        int
	compactExp    int
}

func decimalFromFloat(v float64) decimal {
	d := decimal{minInt: 1}
	if math.IsNaN(v) {
		d.nan = true
		return d
	}
	d.neg = math.Signbit(v)
	if math.IsInf(v, 0) {
		d.inf = true
		return d
	}
	if v == 0 {
		return d
	}
	s := strconv.FormatFloat(math.Abs(v), 'e', -1, 64)
	mant, expText, _ := strings.Cut(s, "e")
	e, _ := strconv.Atoi(expText)
	mant = strings.Replace(mant, ".", "", 1)
	d.digits = []byte(mant)
	d.exp = e - (len(mant) - 1)
	d.normalize()
	return d
}

func (d *decimal) normalize() {
	for len(d.digits) > 0 && d.digits[len(d.digits)-1] == '0' {
		d.digits = d.digits[:len(d.digits)-1]
		d.exp++
	}
	i := 0
	for i < len(d.digits) && d.digits[i] == '0' {
		i++
	}
	d.digits = d.digits[i:]
	if len(d.digits) == 0 {
		d.exp = 0
	}
}

func (d *decimal) isZero() bool { return len(d.digits) == 0 && !d.nan && !d.inf }

// magnitude is the power of ten of the first digit.
func (d *decimal) magnitude() int {
	if len(d.digits) == 0 {
		return 0
	}
	return d.exp + len(d.digits) - 1
}

func (d *decimal) adjust(by int) {
	if len(d.digits) > 0 {
		d.exp += by
	}
}

func (d *decimal) digitAt(pos int) byte {
	i := pos - d.exp
	if i < 0 || i >= len(d.digits) {
		return '0'
	}
	return d.digits[len(d.digits)-1-i]
}

// roundAt rounds to a multiple of inc × 10^pos under the mode.
func (d *decimal) roundAt(pos int, inc int, mode string) {
	if d.nan || d.inf || len(d.digits) == 0 {
		return
	}
	if inc <= 0 {
		inc = 1
	}
	// Work on the integer q = floor(|x| / (inc × 10^pos)) and whether the remainder is below, at or above half.
	whole, rem := d.divideByIncrement(pos, inc)
	cmp := rem // -2 exact, -1 below half, 0 half, 1 above half
	up := false
	if cmp != -2 {
		odd := whole[len(whole)-1]%2 == 1
		switch mode {
		case "ceil":
			up = !d.neg
		case "floor":
			up = d.neg
		case "expand":
			up = true
		case "trunc":
			up = false
		case "halfCeil":
			up = cmp > 0 || cmp == 0 && !d.neg
		case "halfFloor":
			up = cmp > 0 || cmp == 0 && d.neg
		case "halfTrunc":
			up = cmp > 0
		case "halfEven":
			up = cmp > 0 || cmp == 0 && odd
		default: // halfExpand
			up = cmp >= 0
		}
	}
	if up {
		whole = addOne(whole)
	}
	result := mulSmall(whole, inc)
	d.digits = result
	d.exp = pos
	d.normalize()
}

// divideByIncrement returns the digits (values 0-9) of floor(|x| / (inc × 10^pos)) and the remainder's class.
func (d *decimal) divideByIncrement(pos, inc int) ([]byte, int) {
	top := d.magnitude()
	var whole []byte
	var rem int64
	for p := top; p >= pos; p-- {
		rem = rem*10 + int64(d.digitAt(p)-'0')
		whole = append(whole, byte(rem/int64(inc)))
		rem %= int64(inc)
	}
	if len(whole) == 0 {
		whole = []byte{0}
	}
	// What is left below pos.
	frac := false
	for p := pos - 1; p >= d.exp; p-- {
		if d.digitAt(p) != '0' {
			frac = true
			break
		}
	}
	// Compare rem.frac... with inc/2.
	twice := rem * 2
	class := -2
	if rem != 0 || frac {
		switch {
		case twice > int64(inc):
			class = 1
		case twice < int64(inc):
			class = -1
			// rem*2 == inc-1 with odd increments cannot occur for the allowed ones; a fraction can reach half only
			// through the next digit.
			if twice+2 > int64(inc) {
				class = compareFractionHalf(d, pos, int64(inc), twice)
			}
		default:
			if frac {
				class = 1
			} else {
				class = 0
			}
		}
	}
	return whole, class
}

func compareFractionHalf(d *decimal, pos int, inc, twice int64) int {
	// 2×(rem + f) against inc with 0 < f < 1 and twice+2 > inc, so twice+1 == inc: compare f with one half.
	if twice+1 != inc {
		return -1
	}
	first := d.digitAt(pos - 1)
	switch {
	case first > '5':
		return 1
	case first < '5':
		return -1
	}
	for p := pos - 2; p >= d.exp; p-- {
		if d.digitAt(p) != '0' {
			return 1
		}
	}
	return 0
}

func addOne(digits []byte) []byte {
	out := append([]byte(nil), digits...)
	for i := len(out) - 1; i >= 0; i-- {
		if out[i] < 9 {
			out[i]++
			return out
		}
		out[i] = 0
	}
	return append([]byte{1}, out...)
}

// mulSmall multiplies a digit-valued number by n and returns ASCII digits.
func mulSmall(digits []byte, n int) []byte {
	out := make([]byte, len(digits)+5)
	carry := 0
	j := len(out) - 1
	for i := len(digits) - 1; i >= 0; i-- {
		v := int(digits[i])*n + carry
		out[j] = byte('0' + v%10)
		carry = v / 10
		j--
	}
	for j >= 0 {
		out[j] = byte('0' + carry%10)
		carry /= 10
		j--
	}
	return out
}

// operands computes the plural operands of the quantity as displayed.
func (d *decimal) operands() operands {
	var o operands
	lo := d.exp
	if len(d.digits) == 0 {
		lo = 0
	}
	v := 0
	if lo < 0 {
		v = -lo
	}
	w := v
	if d.minFrac > v {
		v = d.minFrac
	}
	o.v, o.w = v, w
	var i uint64
	mag := d.magnitude()
	if len(d.digits) == 0 {
		mag = 0
	}
	for p := min(mag, 17); p >= 0; p-- {
		i = i*10 + uint64(d.digitAt(p)-'0')
	}
	o.i = i
	var f, t uint64
	// toFractionLong: reads until the value passes 1e18, so leading zeros cost nothing, and strips what overflow left of t.
	for p := -1; p >= -v && f <= 1e18; p-- {
		f = f*10 + uint64(d.digitAt(p)-'0')
	}
	for p := -1; p >= -w && t <= 1e18; p-- {
		t = t*10 + uint64(d.digitAt(p)-'0')
	}
	for t > 0 && t%10 == 0 {
		t /= 10
	}
	o.f, o.t = f, t
	n := 0.0
	if len(d.digits) > 0 {
		text := string(d.digits) + "e" + strconv.Itoa(d.exp)
		n, _ = strconv.ParseFloat(text, 64)
	}
	o.n = n
	o.e = int64(d.compactExp)
	return o
}

// rounding is the digit configuration of SetNumberFormatDigitOptions.
type rounding struct {
	minInt           int
	minFrac, maxFrac int
	minSig, maxSig   int
	kind             string // "fraction", "significant", "morePrecision", "lessPrecision", "compact"
	mode             string
	increment        int
	stripIfInteger   bool
}

// digitOptions is SetNumberFormatDigitOptions.
func digitOptions(service string, o NumberOptions, mnfdDefault, mxfdDefault int, compact bool) rounding {
	r := rounding{mode: "halfExpand", increment: 1}
	mnid, _ := numberOption("minimumIntegerDigits", o.MinimumIntegerDigits, 1, 21, 1)
	r.minInt = mnid
	inc, _ := numberOption("roundingIncrement", o.RoundingIncrement, 1, 5000, 1)
	switch inc {
	case 1, 2, 5, 10, 20, 25, 50, 100, 200, 250, 500, 1000, 2000, 2500, 5000:
	default:
		rangeError("roundingIncrement value is out of range.")
	}
	r.increment = inc
	if m := stringOption(service, "roundingMode", o.RoundingMode, "ceil", "floor", "expand", "trunc", "halfCeil", "halfFloor", "halfExpand", "halfTrunc", "halfEven"); m != "" {
		r.mode = m
	}
	priority := stringOption(service, "roundingPriority", o.RoundingPriority, "auto", "morePrecision", "lessPrecision")
	if priority == "" {
		priority = "auto"
	}
	r.stripIfInteger = stringOption(service, "trailingZeroDisplay", o.TrailingZeroDisplay, "auto", "stripIfInteger") == "stripIfInteger"
	if inc != 1 {
		mxfdDefault = mnfdDefault
	}
	hasSd := o.MinimumSignificantDigits != nil || o.MaximumSignificantDigits != nil
	hasFd := o.MinimumFractionDigits != nil || o.MaximumFractionDigits != nil
	needSd, needFd := true, true
	if priority == "auto" {
		needSd = hasSd
		if needSd || !hasFd && compact {
			needFd = false
		}
	}
	if needSd {
		if hasSd {
			r.minSig, _ = numberOption("minimumSignificantDigits", o.MinimumSignificantDigits, 1, 21, 1)
			r.maxSig, _ = numberOption("maximumSignificantDigits", o.MaximumSignificantDigits, r.minSig, 21, 21)
		} else {
			r.minSig, r.maxSig = 1, 21
		}
	}
	if needFd {
		if hasFd {
			mn, hasMn := numberOption("minimumFractionDigits", o.MinimumFractionDigits, 0, 100, 0)
			mx, hasMx := numberOption("maximumFractionDigits", o.MaximumFractionDigits, 0, 100, 0)
			switch {
			case !hasMn:
				mn = min(mnfdDefault, mx)
			case !hasMx:
				mx = max(mxfdDefault, mn)
			case mn > mx:
				rangeError("maximumFractionDigits value is out of range.")
			}
			r.minFrac, r.maxFrac = mn, mx
		} else {
			r.minFrac, r.maxFrac = mnfdDefault, mxfdDefault
		}
	}
	switch {
	case !needSd && !needFd:
		r.minFrac, r.maxFrac, r.minSig, r.maxSig = 0, 0, 1, 2
		r.kind = "compact"
	case priority == "morePrecision" || priority == "lessPrecision":
		r.kind = priority
	case hasSd:
		r.kind = "significant"
	default:
		r.kind = "fraction"
	}
	if inc != 1 {
		if r.kind != "fraction" {
			typeError("RoundingType is not fractionDigits")
		}
		if r.maxFrac != r.minFrac {
			rangeError("maximumFractionDigits value is out of range.")
		}
	}
	return r
}

// apply rounds the quantity and sets its display widths.
func (r rounding) apply(d *decimal) {
	d.minInt = r.minInt
	if d.nan || d.inf {
		return
	}
	switch r.kind {
	case "fraction":
		d.roundAt(-r.maxFrac, r.increment, r.mode)
		d.minFrac = r.minFrac
	case "significant":
		r.applySignificant(d)
	case "compact":
		// Round to an integer, keeping at least two significant digits; the display trims trailing zeros.
		mag := d.magnitude()
		pos := min(mag-1, 0)
		if d.isZero() {
			pos = 0
		}
		before := *d
		d.roundAt(pos, 1, r.mode)
		if d.magnitude() != mag && !d.isZero() {
			mag = d.magnitude()
			*d = before
			d.roundAt(min(mag-1, 0), 1, r.mode)
		}
		d.minFrac = 0
	default:
		more := r.kind == "morePrecision"
		mag := d.magnitude()
		if d.isZero() {
			mag = 0
		}
		sigPos := mag - r.maxSig + 1
		fracPos := -r.maxFrac
		useFrac := fracPos < sigPos
		if !more {
			useFrac = !useFrac
		}
		if useFrac {
			d.roundAt(fracPos, 1, r.mode)
			d.minFrac = r.minFrac
		} else {
			r.applySignificant(d)
		}
	}
	if r.stripIfInteger && d.exp >= 0 {
		d.minFrac = 0
	}
}

func (r rounding) applySignificant(d *decimal) {
	mag := d.magnitude()
	if d.isZero() {
		d.minFrac = max(r.minSig-1, 0)
		return
	}
	pos := mag - r.maxSig + 1
	before := *d
	d.roundAt(pos, 1, r.mode)
	if d.magnitude() != mag {
		mag = d.magnitude()
		*d = before
		d.roundAt(mag-r.maxSig+1, 1, r.mode)
	}
	d.minFrac = max(r.minSig-1-mag, 0)
}
