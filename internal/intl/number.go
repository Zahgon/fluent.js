package intl

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/projectfluent/fluent-go/internal/icudata"
)

// NumberOptions are the options of Intl.NumberFormat and Intl.PluralRules: nil (absent), string, float64 or bool.
type NumberOptions struct {
	LocaleMatcher, NumberingSystem, Style, Currency, CurrencyDisplay, CurrencySign, Unit, UnitDisplay any
	MinimumIntegerDigits, MinimumFractionDigits, MaximumFractionDigits                                any
	MinimumSignificantDigits, MaximumSignificantDigits                                                any
	RoundingIncrement, RoundingMode, RoundingPriority, TrailingZeroDisplay                            any
	Notation, CompactDisplay, UseGrouping, SignDisplay                                                any
	Type                                                                                              any
}

const nfService = "Intl.NumberFormat"

var sanctionedUnits = map[string]bool{}

func init() {
	for _, u := range strings.Fields(`acre bit byte celsius centimeter day degree fahrenheit fluid-ounce foot gallon gigabit
		gigabyte gram hectare hour inch kilobit kilobyte kilogram kilometer liter megabit megabyte meter microsecond mile
		mile-scandinavian milliliter millimeter millisecond minute month nanosecond ounce percent petabyte pound second
		stone terabit terabyte week yard year`) {
		sanctionedUnits[u] = true
	}
}

// NumberFormat is Intl.NumberFormat.
type NumberFormat struct {
	locale               string
	ns                   string
	digits               []rune
	style                string
	currency             string
	currencyDisp         string
	accounting           bool
	unit, perUnit        string
	unitDisplay          string
	notation             string
	compactLong          bool
	grouping             string // "auto", "always", "min2", "false"
	signDisplay          string
	round                rounding
	symbols              map[string]any
	plural               []pluralRule
	pattern              numberPattern
	compact              *compactData
	unitForms            map[string]string
	decimalSym, groupSym string
}

func numberingSystems() map[string]any { return icudata.Table("supp/numberingSystems") }

func validNumberingSystem(name string) bool {
	ns, ok := numberingSystems()[name].(map[string]any)
	if !ok {
		return false
	}
	alg, _ := ns["algorithmic"].(float64)
	return alg == 0
}

func wellFormedType(s string) bool {
	if s == "" {
		return false
	}
	for _, p := range strings.Split(s, "-") {
		if len(p) < 3 || len(p) > 8 || !isASCIIAlnum(p) {
			return false
		}
	}
	return true
}

func wellFormedUnit(unit string) bool {
	if sanctionedUnits[unit] {
		return true
	}
	num, den, ok := strings.Cut(unit, "-per-")
	return ok && sanctionedUnits[num] && sanctionedUnits[den] && !strings.Contains(den, "-per-")
}

func currencyDigits(code string) int {
	meta := icudata.Table("supp/currencyMeta")
	entry, ok := meta[code].([]any)
	if !ok {
		entry, _ = meta["DEFAULT"].([]any)
	}
	if len(entry) == 0 {
		return 2
	}
	d, _ := entry[0].(float64)
	return int(d)
}

// NewNumberFormat is new Intl.NumberFormat(locales, opts).
func NewNumberFormat(locales []string, o NumberOptions) *NumberFormat {
	r := resolveLocale(locales, "nf")
	stringOption(nfService, "localeMatcher", o.LocaleMatcher, "lookup", "best fit")
	nu := ""
	if o.NumberingSystem != nil {
		nu = jsToString(o.NumberingSystem)
		if !wellFormedType(nu) {
			rangeError("Invalid numberingSystem : " + nu)
		}
		nu = strings.ToLower(nu)
	}
	f := &NumberFormat{locale: r.icuName}
	if nu == "" || !validNumberingSystem(nu) {
		nu = r.keyword("nu")
	}
	if !validNumberingSystem(nu) {
		nu = lookupString("main", r.icuName, "NumberElements", "default")
		if nu == "" {
			nu = "latn"
		}
	}
	f.ns = nu
	ns, _ := numberingSystems()[nu].(map[string]any)
	digits, _ := ns["digits"].(string)
	f.digits = []rune(digits)

	f.style = stringOption(nfService, "style", o.Style, "decimal", "percent", "currency", "unit")
	if f.style == "" {
		f.style = "decimal"
	}
	currency := ""
	if o.Currency != nil {
		currency = jsToString(o.Currency)
		if len(currency) != 3 || !isASCIIAlpha(currency) {
			rangeError("Invalid currency code : " + currency)
		}
		currency = strings.ToUpper(currency)
	} else if f.style == "currency" {
		typeError("Currency code is required with currency style.")
	}
	f.currencyDisp = stringOption(nfService, "currencyDisplay", o.CurrencyDisplay, "code", "symbol", "narrowSymbol", "name")
	if f.currencyDisp == "" {
		f.currencyDisp = "symbol"
	}
	f.accounting = stringOption(nfService, "currencySign", o.CurrencySign, "standard", "accounting") == "accounting"
	accountingSign := f.accounting
	unit := ""
	if o.Unit != nil {
		unit = jsToString(o.Unit)
		if !wellFormedUnit(unit) {
			rangeError("Invalid unit argument for Intl.NumberFormat() '" + unit + "'")
		}
	} else if f.style == "unit" {
		typeError("Invalid unit argument for Intl.NumberFormat() ''")
	}
	f.unitDisplay = stringOption(nfService, "unitDisplay", o.UnitDisplay, "short", "narrow", "long")
	if f.unitDisplay == "" {
		f.unitDisplay = "short"
	}
	if f.style == "currency" {
		f.currency = currency
	}
	if f.style == "unit" {
		f.unit = unit
		if num, den, ok := strings.Cut(unit, "-per-"); ok {
			f.unit, f.perUnit = num, den
		}
	}
	f.notation = stringOption(nfService, "notation", o.Notation, "standard", "scientific", "engineering", "compact")
	if f.notation == "" {
		f.notation = "standard"
	}
	mnfd, mxfd := 0, 3
	switch {
	case f.style == "currency" && f.notation == "standard":
		mnfd = currencyDigits(f.currency)
		mxfd = mnfd
	case f.style == "percent":
		mxfd = 0
	}
	f.round = digitOptions(nfService, o, mnfd, mxfd, f.notation == "compact")
	f.compactLong = stringOption(nfService, "compactDisplay", o.CompactDisplay, "short", "long") == "long"
	f.grouping = groupingOption(o.UseGrouping, f.notation == "compact")
	f.signDisplay = stringOption(nfService, "signDisplay", o.SignDisplay, "auto", "never", "always", "exceptZero", "negative")
	if f.signDisplay == "" {
		f.signDisplay = "auto"
	}
	f.accounting = accountingSign && f.signDisplay != "never"
	f.symbols = merged("main", f.locale, "NumberElements", f.ns, "symbols")
	if f.ns != "latn" {
		for k, v := range merged("main", f.locale, "NumberElements", "latn", "symbols") {
			if _, have := f.symbols[k]; !have {
				f.symbols[k] = v
			}
		}
	}
	f.plural = pluralRulesFor(f.locale, false)
	f.decimalSym, f.groupSym = f.symbol("decimal"), f.symbol("group")
	selected := f.selectPattern()
	if f.style == "currency" {
		if s := f.symbol("currencyDecimal"); s != "" {
			f.decimalSym = s
		}
		if s := f.symbol("currencyGroup"); s != "" {
			f.groupSym = s
		}
		// DecimalFormatSymbols first takes the format of the locale's own currency, then that of the currency in use.
		for _, code := range []string{regionCurrency(r), f.currency} {
			if p, dec, group := currencyFormatOf(r.icuName, code); p != "" {
				selected, f.decimalSym, f.groupSym = p, dec, group
			}
		}
	}
	f.pattern = parseNumberPattern(selected)
	if f.notation == "compact" {
		f.compact = loadCompact(f.locale, f.ns, f.compactLong, f.style == "currency" && f.currencyDisp != "name")
	}
	switch {
	case f.style == "percent" && f.notation == "compact":
		f.unitForms = longUnitForms(f.locale, "percent", "", "short")
	case f.style == "unit" && (f.unit != "percent" || f.unitDisplay == "long" || f.notation == "compact"):
		f.unitForms = longUnitForms(f.locale, f.unit, f.perUnit, f.unitDisplay)
	}
	return f
}

// regionCurrency is ucurr_forLocale: the -u-cu- keyword or the legal tender of the locale's region.
func regionCurrency(r resolved) string {
	if code := r.keyword("cu"); len(code) == 3 {
		return strings.ToUpper(code)
	}
	if r.id.region == "" {
		return ""
	}
	code, _ := icudata.Table("supp/currencyMap")[r.id.region].(string)
	return code
}

// currencyFormatOf returns the pattern and monetary separators a Currencies entry carries as its third element.
func currencyFormatOf(locale, code string) (pattern, decimalSep, groupSep string) {
	if code == "" {
		return "", "", ""
	}
	entry, _ := lookup("curr", locale, "Currencies", code).([]any)
	if len(entry) < 3 {
		return "", "", ""
	}
	extra := toStrings(entry[2])
	if len(extra) < 3 {
		return "", "", ""
	}
	return extra[0], extra[1], extra[2]
}

func groupingOption(v any, compact bool) string {
	fallback := "auto"
	if compact {
		fallback = "min2"
	}
	if v == nil {
		return fallback
	}
	if b, ok := v.(bool); ok {
		if b {
			return "always"
		}
		return "false"
	}
	if !jsToBoolean(v) {
		return "false"
	}
	s := jsToString(v)
	switch s {
	case "min2", "auto", "always":
		return s
	case "true", "false":
		return fallback
	}
	rangeError("Value " + s + " out of range for " + nfService + " options property useGrouping")
	return ""
}

func (f *NumberFormat) patternResource(key string) string {
	if p := lookupString("main", f.locale, "NumberElements", f.ns, "patterns", key); p != "" {
		return p
	}
	return lookupString("main", f.locale, "NumberElements", "latn", "patterns", key)
}

func (f *NumberFormat) selectPattern() string {
	switch f.style {
	case "percent":
		if f.notation == "compact" {
			return f.patternResource("decimalFormat")
		}
		return f.patternResource("percentFormat")
	case "currency":
		if f.currencyDisp == "name" {
			return f.patternResource("decimalFormat")
		}
		if f.accounting {
			return f.patternResource("accountingFormat")
		}
		return f.patternResource("currencyFormat")
	case "unit":
		if f.unit == "percent" && f.unitDisplay != "long" && f.notation != "compact" {
			return f.patternResource("percentFormat")
		}
	}
	return f.patternResource("decimalFormat")
}

// numberPattern is a parsed number pattern. Affixes keep their pattern syntax.
type numberPattern struct {
	posPrefix, posSuffix string
	negPrefix, negSuffix string
	hasNeg               bool
	group1, group2       int
}

func parseNumberPattern(s string) numberPattern {
	var p numberPattern
	first, second, hasSecond := cutUnquoted(s, ';')
	var body string
	p.posPrefix, body, p.posSuffix = splitAffixes(first)
	if i := strings.IndexByte(body, '.'); i >= 0 {
		body = body[:i]
	}
	if last := strings.LastIndexByte(body, ','); last >= 0 {
		p.group1 = len(body) - last - 1
		rest := body[:last]
		if prev := strings.LastIndexByte(rest, ','); prev >= 0 {
			p.group2 = last - prev - 1
		} else {
			p.group2 = p.group1
		}
	}
	if hasSecond {
		p.hasNeg = true
		p.negPrefix, _, p.negSuffix = splitAffixes(second)
	}
	return p
}

func cutUnquoted(s string, sep byte) (string, string, bool) {
	quoted := false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\'':
			quoted = !quoted
		case sep:
			if !quoted {
				return s[:i], s[i+1:], true
			}
		}
	}
	return s, "", false
}

func splitAffixes(s string) (prefix, body, suffix string) {
	quoted := false
	start, end := -1, -1
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\'' {
			quoted = !quoted
			continue
		}
		if quoted {
			continue
		}
		if c == '#' || c == '0' || c == ',' || c == '.' || c == '@' || c >= '1' && c <= '9' {
			if start < 0 {
				start = i
			}
			end = i + 1
		}
	}
	if start < 0 {
		return s, "", ""
	}
	return s[:start], s[start:end], s[end:]
}

func (f *NumberFormat) symbol(name string) string {
	s, _ := f.symbols[name].(string)
	return s
}

// affixText expands a pattern affix; lastCurrency and firstCurrency report whether its ends are the currency symbol.
func (f *NumberFormat) affixText(affix string) (text string, firstCurrency, lastCurrency bool) {
	var b strings.Builder
	runes := []rune(affix)
	quoted := false
	last := false
	for i := 0; i < len(runes); i++ {
		c := runes[i]
		if c == '\'' {
			if i+1 < len(runes) && runes[i+1] == '\'' {
				b.WriteRune('\'')
				i++
				last = false
				continue
			}
			quoted = !quoted
			continue
		}
		if quoted {
			b.WriteRune(c)
			last = false
			continue
		}
		switch c {
		case '-':
			b.WriteString(f.symbol("minusSign"))
			last = false
		case '+':
			b.WriteString(f.symbol("plusSign"))
			last = false
		case '%':
			b.WriteString(f.symbol("percentSign"))
			last = false
		case '‰':
			b.WriteString(f.symbol("perMille"))
			last = false
		case '¤':
			for i+1 < len(runes) && runes[i+1] == '¤' {
				i++
			}
			if b.Len() == 0 {
				firstCurrency = true
			}
			b.WriteString(f.currencySymbolText())
			last = true
		default:
			b.WriteRune(c)
			last = false
		}
	}
	return b.String(), firstCurrency, last
}

func (f *NumberFormat) currencySymbolText() string {
	switch f.currencyDisp {
	case "code":
		return f.currency
	case "narrowSymbol":
		if s := lookupString("curr", f.locale, "Currencies%narrow", f.currency); s != "" {
			return s
		}
	}
	if v := lookupStrings("curr", f.locale, "Currencies", f.currency); len(v) > 0 {
		return v[0]
	}
	return f.currency
}

func (f *NumberFormat) currencyName(plural string) string {
	if s := lookupString("curr", f.locale, "CurrencyPlurals", f.currency, plural); s != "" {
		return s
	}
	if s := lookupString("curr", f.locale, "CurrencyPlurals", f.currency, "other"); s != "" {
		return s
	}
	if v := lookupStrings("curr", f.locale, "Currencies", f.currency); len(v) > 1 {
		return v[1]
	}
	return f.currency
}

func (f *NumberFormat) currencyUnitPattern(plural string) string {
	if s := lookupString("curr", f.locale, "CurrencyUnitPatterns", plural); s != "" {
		return s
	}
	return lookupString("curr", f.locale, "CurrencyUnitPatterns", "other")
}

// signum classes.
const (
	signPos = iota
	signPosZero
	signNegZero
	signNeg
)

func (d *decimal) signum() int {
	switch {
	case d.nan:
		return signPosZero
	case d.isZero():
		if d.neg {
			return signNegZero
		}
		return signPosZero
	case d.neg:
		return signNeg
	}
	return signPos
}

// patternSignType: 0 positive, 1 positive with a plus sign, 2 negative.
func (f *NumberFormat) patternSign(signum int) int {
	switch f.signDisplay {
	case "always":
		if signum == signNeg || signum == signNegZero {
			return 2
		}
		return 1
	case "exceptZero":
		switch signum {
		case signNeg:
			return 2
		case signPos:
			return 1
		}
		return 0
	case "negative":
		if signum == signNeg {
			return 2
		}
		return 0
	case "never":
		return 0
	}
	if signum == signNeg || signum == signNegZero {
		return 2
	}
	return 0
}

func patternAffixes(p numberPattern, sign int) (prefix, suffix string) {
	positiveHasPlus := strings.Contains(p.posPrefix, "+") || strings.Contains(p.posSuffix, "+")
	negativeHasMinus := strings.Contains(p.negPrefix, "-") || strings.Contains(p.negSuffix, "-")
	plusReplaces := sign == 1 && !positiveHasPlus
	useNeg := p.hasNeg && (sign == 2 || negativeHasMinus && plusReplaces)
	if useNeg {
		prefix, suffix = p.negPrefix, p.negSuffix
	} else {
		prefix, suffix = p.posPrefix, p.posSuffix
	}
	if !useNeg && (sign == 2 || plusReplaces) {
		if plusReplaces {
			prefix = "+" + prefix
		} else {
			prefix = "-" + prefix
		}
	}
	if plusReplaces {
		prefix = replaceUnquoted(prefix, '-', '+')
		suffix = replaceUnquoted(suffix, '-', '+')
	}
	return prefix, suffix
}

func replaceUnquoted(s string, from, to byte) string {
	b := []byte(s)
	quoted := false
	for i := range b {
		if b[i] == '\'' {
			quoted = !quoted
		} else if !quoted && b[i] == from {
			b[i] = to
		}
	}
	return string(b)
}

// Format is format(value).
func (f *NumberFormat) Format(value float64) string {
	d := decimalFromFloat(value)
	if f.style == "percent" && !d.nan && !d.inf {
		d.adjust(2)
	}
	original := d
	noBody := false
	exponent := 0
	pattern := f.pattern
	switch f.notation {
	case "scientific", "engineering":
		exponent = f.applyScientific(&d)
	case "compact":
		pat := f.applyCompact(&d)
		if pat != "" {
			noBody = contiguousZeros(pat) == 0
			pattern = parseNumberPattern(pat)
			pattern.group1, pattern.group2 = f.pattern.group1, f.pattern.group2
		}
	default:
		f.round.apply(&d)
	}
	sign := f.patternSign(d.signum())
	body := f.renderNumber(&d, f.pattern, f.notation == "scientific" || f.notation == "engineering", exponent)
	prefix, suffix := patternAffixes(pattern, sign)
	preText, _, preCurrency := f.affixText(prefix)
	sufText, sufCurrency, _ := f.affixText(suffix)
	inserted := 0
	if preCurrency {
		spaced := f.spaceCurrency(preText, body, true)
		inserted += utf16Len(spaced) - utf16Len(preText)
		preText = spaced
	}
	if sufCurrency {
		spaced := f.spaceCurrency(sufText, body, false)
		inserted += utf16Len(spaced) - utf16Len(sufText)
		sufText = spaced
	}
	if noBody {
		body = ""
	}
	out := preText + body + sufText
	if f.style == "currency" && f.currencyDisp == "name" {
		plural := f.longNamePlural(original, &d, exponent)
		// ICU does not count the characters currency spacing inserted, so the long name's right side lands that many
		// UTF-16 units early.
		out = applyLongName(f.currencyUnitPattern(plural), out, f.currencyName(plural), inserted)
	}
	if f.unitForms != nil {
		plural := f.longNamePlural(original, &d, exponent)
		form, ok := f.unitForms[plural]
		if !ok {
			form = f.unitForms["other"]
		}
		out = applySimple(form, out, "")
	}
	return out
}

// longNamePlural selects a long name's plural form. Compact and standard notation: ICU's handler runs before notation
// scaling, on a rounded copy of the quantity. Scientific and engineering: the scientific handler has already rounded the
// quantity and recorded the exponent on it, so the operands are the rounded value with its exponent, and e is that exponent.
func (f *NumberFormat) longNamePlural(q decimal, scaled *decimal, exponent int) string {
	if q.nan || q.inf {
		return "other"
	}
	if f.notation == "scientific" || f.notation == "engineering" {
		q = *scaled
		q.adjust(exponent)
		q.minFrac -= exponent
		o := q.operands()
		o.e = int64(exponent)
		return selectPlural(f.plural, o)
	}
	f.round.apply(&q)
	return selectPlural(f.plural, q.operands())
}

func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n++
		if r >= 0x10000 {
			n++
		}
	}
	return n
}

// applyLongName wraps out in a two-argument pattern, its text after {0} placed short units before out's end.
func applyLongName(pattern, out, name string, short int) string {
	text := strings.ReplaceAll(pattern, "{1}", name)
	before, after, ok := strings.Cut(text, "{0}")
	if !ok {
		return text
	}
	if short == 0 {
		return before + out + after
	}
	u := utf16.Encode([]rune(out))
	cut := len(u) - short
	return before + string(utf16.Decode(u[:cut])) + after + string(utf16.Decode(u[cut:]))
}

// applySimple fills {0} and {1} of a simple pattern; a pattern with no {0} drops the number.
func applySimple(pattern, zero, one string) string {
	out := strings.ReplaceAll(pattern, "{0}", zero)
	return strings.ReplaceAll(out, "{1}", one)
}

// spaceCurrency applies the currencySpacing rule between a currency symbol and the number.
func (f *NumberFormat) spaceCurrency(affix, body string, isPrefix bool) string {
	key := "beforeCurrency"
	if isPrefix {
		key = "afterCurrency"
	}
	insert := lookupString("curr", f.locale, "currencySpacing", key, "insertBetween")
	if insert == "" {
		return affix
	}
	var adjacent, numberEdge rune
	if isPrefix {
		adjacent, _ = utf8.DecodeLastRuneInString(affix)
		numberEdge, _ = utf8.DecodeRuneInString(body)
	} else {
		adjacent, _ = utf8.DecodeRuneInString(affix)
		numberEdge, _ = utf8.DecodeLastRuneInString(body)
	}
	if unicode.Is(unicode.S, adjacent) || unicode.Is(unicode.Z, adjacent) || !unicode.Is(unicode.Nd, numberEdge) {
		return affix
	}
	if isPrefix {
		return affix + insert
	}
	return insert + affix
}

func (f *NumberFormat) minGrouping() int {
	if s := lookupString("main", f.locale, "NumberElements", "minimumGroupingDigits"); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			return n
		}
	}
	return 1
}

// renderNumber writes the digits with grouping and separators, then any exponent.
func (f *NumberFormat) renderNumber(d *decimal, p numberPattern, scientific bool, exponent int) string {
	if d.nan {
		return f.symbol("nan")
	}
	if d.inf {
		return f.symbol("infinity")
	}
	decimalSym, groupSym := f.decimalSym, f.groupSym
	upper := d.magnitude()
	if len(d.digits) == 0 || upper < 0 {
		upper = 0
	}
	if m := d.minInt - 1; m > upper {
		upper = m
	}
	group1, group2 := p.group1, p.group2
	minGroup := 1
	switch f.grouping {
	case "false":
		group1 = 0
	case "always":
		minGroup = 1
	case "min2":
		minGroup = max(f.minGrouping(), 2)
	default:
		minGroup = f.minGrouping()
		if f.notation == "compact" {
			minGroup = max(minGroup, 2)
		}
	}
	var b strings.Builder
	for pos := upper; pos >= 0; pos-- {
		b.WriteRune(f.digitRune(d.digitAt(pos)))
		if group1 > 0 && pos > 0 && pos >= group1 && (pos-group1)%group2 == 0 && upper-group1+1 >= minGroup {
			b.WriteString(groupSym)
		}
	}
	lower := d.exp
	if len(d.digits) == 0 || lower > 0 {
		lower = 0
	}
	if m := -d.minFrac; m < lower {
		lower = m
	}
	if lower < 0 {
		b.WriteString(decimalSym)
		for pos := -1; pos >= lower; pos-- {
			b.WriteRune(f.digitRune(d.digitAt(pos)))
		}
	}
	if scientific {
		b.WriteString(f.symbol("exponential"))
		e := exponent
		if e < 0 {
			b.WriteString(f.symbol("minusSign"))
			e = -e
		}
		b.WriteString(f.exponentDigits(e))
	}
	return b.String()
}

// exponentDigits writes the exponent as ICU does, least significant digit first, each inserted j UTF-16 units before
// the end: with a numbering system outside the BMP that splits surrogate pairs, which then print as U+FFFD.
func (f *NumberFormat) exponentDigits(e int) string {
	var out []uint16
	i := 0
	for j := 0; j < 1 || e > 0; j++ {
		unit := utf16.Encode([]rune{f.digitRune(byte('0' + e%10))})
		at := i - j
		out = append(out[:at], append(unit, out[at:]...)...)
		i += len(unit)
		e /= 10
	}
	return string(utf16.Decode(out))
}

func (f *NumberFormat) digitRune(c byte) rune {
	if len(f.digits) == 10 {
		return f.digits[c-'0']
	}
	return rune(c)
}

// chooseMultiplier applies ICU's chooseMultiplierAndApply: pick a multiplier by magnitude, round, and redo it when
// rounding carried the quantity into a magnitude with another multiplier.
func (f *NumberFormat) chooseMultiplier(d *decimal, multiplierFor func(int) int) int {
	mag := d.magnitude()
	m := multiplierFor(mag)
	orig := *d
	d.adjust(m)
	f.round.apply(d)
	if d.isZero() {
		return m
	}
	if d.magnitude() == mag+m {
		return m
	}
	m2 := multiplierFor(mag + 1)
	if m2 == m {
		return m
	}
	*d = orig
	d.adjust(m2)
	f.round.apply(d)
	return m2
}

func (f *NumberFormat) applyScientific(d *decimal) int {
	if d.nan || d.inf || d.isZero() {
		f.round.apply(d)
		return 0
	}
	interval := 1
	if f.notation == "engineering" {
		interval = 3
	}
	m := f.chooseMultiplier(d, func(magnitude int) int {
		shown := 1
		if interval > 1 {
			shown = (magnitude%interval+interval)%interval + 1
		}
		return shown - magnitude - 1
	})
	return -m
}

type compactData struct {
	multipliers map[int]int
	patterns    map[int]map[string]string
	largest     int
}

// merge folds one compact table over the locale's chain: a more specific bundle's plural form wins, and the
// first pattern with zeros seen for a magnitude fixes its multiplier.
func (c *compactData) merge(locale, ns, style, kind string) {
	path := []string{"NumberElements", ns, style, kind}
	for depth := 0; depth < 8; depth++ {
		var redirect []string
		for _, name := range chain("main", locale) {
			r := walk(icudata.Table("main/"+name), path)
			if r.alias != "" {
				if target, ok := aliasTarget(r.alias); ok {
					redirect = target
					break
				}
				return
			}
			table, ok := r.value.(map[string]any)
			if !ok {
				continue
			}
			for _, key := range sortedKeys(table) {
				variants, ok := table[key].(map[string]any)
				if !ok {
					continue
				}
				mag := len(key) - 1
				if c.patterns[mag] == nil {
					c.patterns[mag] = map[string]string{}
				}
				multiplier := c.multipliers[mag]
				for _, plural := range sortedKeys(variants) {
					if _, have := c.patterns[mag][plural]; have {
						continue
					}
					text, _ := variants[plural].(string)
					if text == "0" {
						c.patterns[mag][plural] = ""
						continue
					}
					c.patterns[mag][plural] = text
					if multiplier == 0 {
						if zeros := contiguousZeros(text); zeros > 0 {
							multiplier = zeros - mag - 1
						}
					}
				}
				if c.multipliers[mag] == 0 {
					c.multipliers[mag] = multiplier
					if mag > c.largest {
						c.largest = mag
					}
				}
			}
		}
		if redirect == nil {
			return
		}
		path = redirect
	}
}

func loadCompact(locale, ns string, long, currency bool) *compactData {
	kind := "decimalFormat"
	if currency {
		kind = "currencyFormat"
	}
	style := "patternsShort"
	if long {
		style = "patternsLong"
	}
	c := &compactData{multipliers: map[int]int{}, patterns: map[int]map[string]string{}}
	c.merge(locale, ns, style, kind)
	if len(c.patterns) == 0 && ns != "latn" {
		c.merge(locale, "latn", style, kind)
	}
	if len(c.patterns) == 0 && long {
		c.merge(locale, ns, "patternsShort", kind)
	}
	if len(c.patterns) == 0 && ns != "latn" && long {
		c.merge(locale, "latn", "patternsShort", kind)
	}
	return c
}

func contiguousZeros(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '0' {
			n++
		} else if n > 0 {
			break
		}
	}
	return n
}

func (c *compactData) multiplier(magnitude int) int {
	if magnitude < 0 {
		return 0
	}
	if magnitude > c.largest {
		magnitude = c.largest
	}
	return c.multipliers[magnitude]
}

func (c *compactData) pattern(magnitude int, exact, plural string) string {
	if magnitude < 0 {
		return ""
	}
	if magnitude > c.largest {
		magnitude = c.largest
	}
	forms := c.patterns[magnitude]
	if forms == nil {
		return ""
	}
	if exact != "" {
		if p, ok := forms[exact]; ok {
			return p
		}
	}
	if p, ok := forms[plural]; ok {
		return p
	}
	return forms["other"]
}

// applyCompact rounds into compact form and returns the compact pattern ("" for none).
func (f *NumberFormat) applyCompact(d *decimal) string {
	magnitude, m := 0, 0
	if d.nan || d.inf || d.isZero() {
		f.round.apply(d)
	} else {
		m = f.chooseMultiplier(d, f.compact.multiplier)
		if !d.isZero() {
			magnitude = d.magnitude()
		}
		magnitude -= m
	}
	plural := "other"
	exact := ""
	if !d.nan && !d.inf {
		plural = selectPlural(f.plural, d.operands())
		switch {
		case d.isZero():
			exact = "0"
		case !d.neg && len(d.digits) == 1 && d.digits[0] == '1' && d.exp == 0:
			exact = "1"
		}
	}
	return f.compact.pattern(magnitude, exact, plural)
}

var unitTypeCache map[string]string

// unitType finds the resource type of a built-in unit from the root unit table of English.
func unitType(unit string) string {
	if unitTypeCache == nil {
		unitTypeCache = map[string]string{}
		units := merged("unit", "en", "units")
		for _, typ := range sortedKeys(units) {
			if t, ok := units[typ].(map[string]any); ok {
				for sub := range t {
					if _, seen := unitTypeCache[sub]; !seen {
						unitTypeCache[sub] = typ
					}
				}
			}
		}
	}
	return unitTypeCache[unit]
}

func unitWidthKey(display string) string {
	switch display {
	case "narrow":
		return "unitsNarrow"
	case "long":
		return "units"
	}
	return "unitsShort"
}

func unitData(locale, unit, display string) map[string]any {
	typ := unitType(unit)
	if typ == "" {
		return nil
	}
	return merged("unit", locale, unitWidthKey(display), typ, unit)
}

// longUnitForms computes the plural forms of a unit, compounded with a per-unit if there is one.
func longUnitForms(locale, unit, perUnit, display string) map[string]string {
	if perUnit != "" {
		if combined := unitData(locale, unit+"-per-"+perUnit, display); len(combined) > 0 {
			unit, perUnit = unit+"-per-"+perUnit, ""
		}
	}
	primary := unitData(locale, unit, display)
	forms := map[string]string{}
	var per string
	if perUnit != "" {
		secondary := unitData(locale, perUnit, display)
		if p, ok := secondary["per"].(string); ok {
			per = p
		} else {
			raw := lookupString("unit", locale, unitWidthKey(display), "compound", "per")
			one, _ := secondary["one"].(string)
			if one == "" {
				one, _ = secondary["other"].(string)
			}
			one = strings.TrimSpace(strings.ReplaceAll(one, "{0}", ""))
			per = strings.ReplaceAll(strings.ReplaceAll(raw, "{0}", "\x00"), "{1}", one)
			per = strings.ReplaceAll(per, "\x00", "{0}")
		}
	}
	for plural, v := range primary {
		text, ok := v.(string)
		if !ok || plural == "dnam" || plural == "per" || plural == "gender" {
			continue
		}
		if per != "" {
			text = strings.ReplaceAll(per, "{0}", text)
		}
		forms[plural] = text
	}
	return forms
}
