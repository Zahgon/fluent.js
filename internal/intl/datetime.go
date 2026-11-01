package intl

import (
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/projectfluent/fluent-go/internal/icudata"
)

// DateTimeOptions are the options of Intl.DateTimeFormat: nil (absent), string, float64 or bool.
type DateTimeOptions struct {
	LocaleMatcher, Calendar, NumberingSystem, Hour12, HourCycle, TimeZone any
	Weekday, Era, Year, Month, Day, DayPeriod, Hour, Minute, Second       any
	FractionalSecondDigits, TimeZoneName, FormatMatcher                   any
	DateStyle, TimeStyle                                                  any
}

const dtfService = "Intl.DateTimeFormat"

// DateTimeFormat is Intl.DateTimeFormat.
type DateTimeFormat struct {
	locale      string
	calendar    string // BCP 47 name
	resource    string // the calendar's name in the resource bundles
	ns          string
	digitPrefix string
	digits      []rune
	zone        *timeZone
	pattern     string
	numbers     string // the DateTimePatterns numbers override, when the pattern has one
	hasMin      bool
	hasSec      bool
	region      string
	langTag     string
}

var calendarResources = map[string]string{
	"gregory": "gregorian", "buddhist": "buddhist", "chinese": "chinese", "coptic": "coptic", "dangi": "dangi", "ethioaa": "ethiopic-amete-alem",
	"ethiopic": "ethiopic", "hebrew": "hebrew", "indian": "indian", "islamic": "islamic", "islamic-civil": "islamic-civil", "islamic-rgsa": "islamic-rgsa",
	"islamic-tbla": "islamic-tbla", "islamic-umalqura": "islamic-umalqura", "iso8601": "iso8601", "japanese": "japanese", "persian": "persian", "roc": "roc",
}

var calendarAliases = map[string]string{"gregorian": "gregory", "ethiopic-amete-alem": "ethioaa", "islamicc": "islamic-civil"}

func normalizeCalendar(name string) string {
	if a, ok := calendarAliases[name]; ok {
		return a
	}
	return name
}

// defaultCalendar is the calendar/default a locale inherits.
func defaultCalendar(r resolved) string {
	if name := lookupString("main", r.icuName, "calendar", "default"); name != "" {
		return normalizeCalendar(name)
	}
	return "gregory"
}

// generatorCalendar is the calendar resource DateTimePatternGenerator uses for a locale with no calendar keyword. It
// follows calendar/default, except that a region under a script bundle holding the default does not inherit it.
func generatorCalendar(icuName string) string {
	names := chain("main", icuName)
	for i, name := range names {
		t, _ := icudata.Table("main/" + name)["calendar"].(map[string]any)
		def, ok := t["default"].(string)
		if !ok {
			continue
		}
		if i > 0 && strings.Count(name, "_") == 1 {
			return "gregorian"
		}
		return def
	}
	return "gregorian"
}

func hourCharFor(r resolved) byte {
	lang, region := r.id.lang, r.id.region
	if region == "" || lang == "" {
		m := maximizeLSR(r.id.lang, r.id.script, r.id.region, "")
		lang, region = m[0], m[2]
	}
	if lang == "" {
		lang = "und"
	}
	if region == "" {
		region = "001"
	}
	timeData, _ := icudata.Table("supp/time")["timeData"].(map[string]any)
	entry, ok := timeData[lang+"_"+region].(map[string]any)
	if !ok {
		entry, ok = timeData[region].(map[string]any)
	}
	if !ok {
		entry, ok = timeData["001"].(map[string]any)
	}
	if !ok {
		return 'H'
	}
	first, _ := entry["preferred"].(string)
	switch first {
	case "h", "H", "K", "k":
		return first[0]
	}
	return 'H'
}

func hourCycleOfChar(c byte) string {
	switch c {
	case 'K':
		return "h11"
	case 'h':
		return "h12"
	case 'k':
		return "h24"
	}
	return "h23"
}

func hourCharOfCycle(hc string) byte {
	switch hc {
	case "h11":
		return 'K'
	case "h12":
		return 'h'
	case "h24":
		return 'k'
	}
	return 'H'
}

// NewDateTimeFormat is new Intl.DateTimeFormat(locales, opts).
func NewDateTimeFormat(locales []string, o DateTimeOptions) *DateTimeFormat {
	r := resolveLocale(locales, "dtf")
	stringOption(dtfService, "localeMatcher", o.LocaleMatcher, "lookup", "best fit")
	calendar := ""
	if o.Calendar != nil {
		calendar = jsToString(o.Calendar)
		if !wellFormedType(calendar) {
			rangeError("Invalid calendar : " + calendar)
		}
		calendar = normalizeCalendar(strings.ToLower(calendar))
	}
	nu := ""
	if o.NumberingSystem != nil {
		nu = jsToString(o.NumberingSystem)
		if !wellFormedType(nu) {
			rangeError("Invalid numberingSystem : " + nu)
		}
		nu = strings.ToLower(nu)
	}
	var hour12 *bool
	if o.Hour12 != nil {
		b := jsToBoolean(o.Hour12)
		hour12 = &b
	}
	hourCycle := stringOption(dtfService, "hourCycle", o.HourCycle, "h11", "h12", "h23", "h24")
	zone := loadZone("Etc/UTC")
	if o.TimeZone != nil {
		zone = resolveTimeZone(jsToString(o.TimeZone))
	} else if z := loadZone("UTC"); z != nil {
		zone = z
	}

	f := &DateTimeFormat{locale: r.icuName, zone: zone, region: r.id.region}
	if _, ok := calendarResources[calendar]; !ok {
		calendar = normalizeCalendar(r.keyword("ca"))
	}
	if _, ok := calendarResources[calendar]; !ok {
		calendar = defaultCalendar(r)
	}
	f.calendar = calendar
	f.resource = calendarResources[calendar]
	if nu == "" || !validNumberingSystem(nu) {
		nu = r.keyword("nu")
	}
	if !validNumberingSystem(nu) {
		nu = lookupString("main", r.icuName, "NumberElements", "default")
	}
	if !validNumberingSystem(nu) {
		nu = "latn"
	}
	f.ns = nu
	ns, _ := numberingSystems()[nu].(map[string]any)
	digits, _ := ns["digits"].(string)
	f.digits = []rune(digits)
	f.digitPrefix = parseNumberPattern(lookupString("main", f.locale, "NumberElements", nu, "patterns", "decimalFormat")).posPrefix

	hourChar := hourCharFor(r)
	localeHC := r.keyword("hc")
	switch localeHC {
	case "h11", "h12", "h23", "h24":
	default:
		localeHC = ""
	}
	generatorChar := hourChar
	if localeHC != "" {
		generatorChar = hourCharOfCycle(localeHC)
	}
	genCalendar := f.resource
	if o.Calendar == nil && r.keyword("ca") == "" || calendarResources[normalizeCalendar(r.keyword("ca"))] == "" && o.Calendar == nil {
		genCalendar = generatorCalendar(f.locale)
	}
	gen := newPatternGenerator(f.locale, genCalendar, f.ns, generatorChar)
	hcDefault := hourCycleOfChar(generatorChar)
	hc := hourCycle
	switch {
	case hour12 != nil && *hour12:
		hc = defaultHourCycle12(r, hcDefault)
	case hour12 != nil:
		hc = defaultHourCycle24(r, hcDefault)
	case hc == "":
		hc = localeHC
		if hc == "" {
			hc = hcDefault
		}
	}

	weekday := stringOption(dtfService, "weekday", o.Weekday, "narrow", "long", "short")
	era := stringOption(dtfService, "era", o.Era, "narrow", "long", "short")
	year := stringOption(dtfService, "year", o.Year, "2-digit", "numeric")
	month := stringOption(dtfService, "month", o.Month, "2-digit", "numeric", "narrow", "long", "short")
	day := stringOption(dtfService, "day", o.Day, "2-digit", "numeric")
	dayPeriod := stringOption(dtfService, "dayPeriod", o.DayPeriod, "narrow", "long", "short")
	hour := stringOption(dtfService, "hour", o.Hour, "2-digit", "numeric")
	minute := stringOption(dtfService, "minute", o.Minute, "2-digit", "numeric")
	second := stringOption(dtfService, "second", o.Second, "2-digit", "numeric")
	fsd := 0
	if o.FractionalSecondDigits != nil {
		fsd, _ = numberOption("fractionalSecondDigits", o.FractionalSecondDigits, 1, 3, 0)
	}
	tzn := stringOption(dtfService, "timeZoneName", o.TimeZoneName, "short", "long", "shortOffset", "longOffset", "shortGeneric", "longGeneric")
	stringOption(dtfService, "formatMatcher", o.FormatMatcher, "basic", "best fit")
	dateStyle := stringOption(dtfService, "dateStyle", o.DateStyle, "full", "long", "medium", "short")
	timeStyle := stringOption(dtfService, "timeStyle", o.TimeStyle, "full", "long", "medium", "short")

	if dateStyle != "" || timeStyle != "" {
		explicit := weekday != "" || era != "" || year != "" || month != "" || day != "" || dayPeriod != "" || hour != "" ||
			minute != "" || second != "" || tzn != "" || fsd != 0
		if explicit {
			typeError("Invalid option : option")
		}
		f.pattern, f.numbers = f.stylePattern(dateStyle, timeStyle, hc, gen, r, localeHC != "")
	} else {
		var sk strings.Builder
		sk.WriteString(map[string]string{"narrow": "EEEEE", "long": "EEEE", "short": "EEE"}[weekday])
		sk.WriteString(map[string]string{"narrow": "GGGGG", "long": "GGGG", "short": "GGG"}[era])
		sk.WriteString(map[string]string{"2-digit": "yy", "numeric": "y"}[year])
		sk.WriteString(map[string]string{"narrow": "MMMMM", "long": "MMMM", "short": "MMM", "2-digit": "MM", "numeric": "M"}[month])
		sk.WriteString(map[string]string{"2-digit": "dd", "numeric": "d"}[day])
		sk.WriteString(map[string]string{"narrow": "BBBBB", "long": "BBBB", "short": "B"}[dayPeriod])
		hourChar := hourCharOfCycle(hc)
		switch hour {
		case "2-digit":
			sk.WriteString(strings.Repeat(string(hourChar), 2))
		case "numeric":
			sk.WriteByte(hourChar)
		}
		sk.WriteString(map[string]string{"2-digit": "mm", "numeric": "m"}[minute])
		sk.WriteString(map[string]string{"2-digit": "ss", "numeric": "s"}[second])
		sk.WriteString(strings.Repeat("S", fsd))
		sk.WriteString(map[string]string{"long": "zzzz", "short": "z", "longOffset": "OOOO", "shortOffset": "O", "longGeneric": "vvvv", "shortGeneric": "v"}[tzn])
		needDefaults := weekday == "" && year == "" && month == "" && day == "" && dayPeriod == "" && hour == "" && minute == "" && second == "" && fsd == 0
		skeleton := sk.String()
		if needDefaults {
			skeleton = strings.Replace(skeleton, map[string]string{"long": "zzzz", "short": "z", "longOffset": "OOOO", "shortOffset": "O", "longGeneric": "vvvv", "shortGeneric": "v"}[tzn], "", 1)
			skeleton = map[string]string{"narrow": "GGGGG", "long": "GGGG", "short": "GGG"}[era] + "yMd" + map[string]string{"long": "zzzz", "short": "z", "longOffset": "OOOO", "shortOffset": "O", "longGeneric": "vvvv", "shortGeneric": "v"}[tzn]
		}
		hasHour := hour != ""
		pattern := gen.bestPattern(skeleton)
		if hasHour {
			pattern = replaceHourCycle(pattern, hc)
		}
		f.pattern = pattern
	}
	f.hasMin = patternHas(f.pattern, 'm')
	f.hasSec = patternHas(f.pattern, 's')
	return f
}

func patternHas(pattern string, letter byte) bool {
	for _, item := range patternItems(pattern) {
		if item[0] == letter {
			return true
		}
	}
	return false
}

func defaultHourCycle12(r resolved, hcDefault string) string {
	if hcDefault == "h11" || hcDefault == "h12" {
		return hcDefault
	}
	if r.id.region == "JP" {
		return "h11"
	}
	return "h12"
}

func defaultHourCycle24(r resolved, hcDefault string) string {
	if hcDefault == "h23" || hcDefault == "h24" {
		return hcDefault
	}
	if r.id.region == "JP" {
		return "h23"
	}
	return "h23"
}

// replaceHourCycle is V8's ReplaceHourCycleInPattern.
func replaceHourCycle(pattern, hc string) string {
	c := hourCharOfCycle(hc)
	b := []byte(pattern)
	quoted := false
	for i := range b {
		switch b[i] {
		case '\'':
			quoted = !quoted
		case 'H', 'h', 'K', 'k':
			if !quoted {
				b[i] = c
			}
		}
	}
	return string(b)
}

var styleIndex = map[string]int{"full": 0, "long": 1, "medium": 2, "short": 3}

// stylePattern is what V8 gets for dateStyle and timeStyle: ICU's style pattern, regenerated through the pattern
// generator when the requested hour cycle differs from the pattern's.
func (f *DateTimeFormat) stylePattern(dateStyle, timeStyle, hc string, gen *patternGenerator, r resolved, localeHC bool) (string, string) {
	patterns, _ := lookup("main", f.locale, "calendar", f.resource, "DateTimePatterns").([]any)
	if len(patterns) < 9 {
		patterns, _ = lookup("main", f.locale, "calendar", "gregorian", "DateTimePatterns").([]any)
	}
	var datePattern, timePattern, numbers string
	if dateStyle != "" {
		item := patterns[4+styleIndex[dateStyle]]
		datePattern = patternText(item)
		if list, ok := item.([]any); ok && len(list) > 1 {
			numbers, _ = list[1].(string)
		}
	}
	if timeStyle != "" {
		timePattern = patternText(patterns[styleIndex[timeStyle]])
		if localeHC || f.validLocaleDiffers(r) {
			skeleton := map[string]string{"full": "mmsszzzz", "long": "mmssz", "medium": "mmss", "short": "mm"}[timeStyle]
			noStd := newPatternGeneratorOptions(f.locale, gen.calendar, f.ns, gen.hourChar, true)
			timePattern = noStd.bestPatternOptions(string(gen.hourChar)+skeleton, false)
		}
	}
	whole := timePattern
	switch {
	case dateStyle != "" && timeStyle != "":
		glue := ""
		at := lookupStrings("main", f.locale, "calendar", f.resource, "DateTimePatterns%atTime")
		if len(at) < 4 {
			at = lookupStrings("main", f.locale, "calendar", "gregorian", "DateTimePatterns%atTime")
		}
		if len(at) >= 4 {
			glue = at[styleIndex[dateStyle]]
		}
		if glue == "" && len(patterns) > 9+styleIndex[dateStyle] {
			glue = patternText(patterns[9+styleIndex[dateStyle]])
		}
		if glue == "" {
			glue = patternText(patterns[8])
		}
		whole = strings.ReplaceAll(strings.ReplaceAll(glue, "{0}", timePattern), "{1}", datePattern)
	case dateStyle != "":
		return datePattern, numbers
	}
	if hc == "" || hc == patternHourCycle(timePattern) {
		return whole, numbers
	}
	sk := parseSkeleton(whole)
	var b strings.Builder
	for field, text := range sk.original {
		switch field {
		case fieldDayPeriod:
		case fieldHour:
			b.WriteString(strings.Repeat(string(hourCharOfCycle(hc)), len(text)))
		default:
			b.WriteString(text)
		}
	}
	return replaceHourCycle(gen.bestPattern(b.String()), hc), ""
}

// validLocaleDiffers is SimpleDateFormat's test for building a time pattern through the generator: the deepest bundle
// found has another language, or lacks the requested region.
func (f *DateTimeFormat) validLocaleDiffers(r resolved) bool {
	names := chain("main", f.locale)
	if len(names) == 0 || names[0] == f.locale {
		return false
	}
	valid := strings.Split(names[0], "_")
	base := strings.Split(f.locale, "_")
	if valid[0] != base[0] {
		return true
	}
	return r.id.region != "" && !slices.Contains(valid[1:], r.id.region)
}

func patternHourCycle(pattern string) string {
	for _, item := range patternItems(pattern) {
		switch item[0] {
		case 'H', 'h', 'K', 'k':
			return hourCycleOfChar(item[0])
		}
	}
	return ""
}

// Format is format(ms).
func (f *DateTimeFormat) Format(ms float64) string {
	if math.IsNaN(ms) || math.Abs(ms) > 8.64e15 {
		rangeError("Invalid time value")
	}
	t := int64(ms)
	out := f.formatPattern(t)
	return strings.ReplaceAll(out, " ", " ")
}

func (f *DateTimeFormat) formatPattern(t int64) string {
	raw, dst := f.zone.offsets(t)
	local := t + raw + dst
	day := floorDiv(local, 86400000)
	millis := local - day*86400000
	fields := calendarFieldsFor(f.calendar, day)
	weekday := int(floorMod(day+4, 7)) + 1 // Sunday is 1
	c := dateContext{t: t, fields: fields, weekday: weekday, millis: millis, raw: raw, dst: dst}
	var out strings.Builder
	for _, item := range patternItems(f.pattern) {
		switch {
		case item[0] == '\'':
			out.WriteString(unquote(item))
		case item[0] >= 'a' && item[0] <= 'z' || item[0] >= 'A' && item[0] <= 'Z':
			out.WriteString(f.field(item[0], len(item), &c))
		default:
			out.WriteString(item)
		}
	}
	return out.String()
}

func unquote(item string) string {
	if item == "''" {
		return "'"
	}
	body := strings.TrimPrefix(item, "'")
	body = strings.TrimSuffix(body, "'")
	return strings.ReplaceAll(body, "''", "'")
}

type dateContext struct {
	t        int64
	fields   calFields
	weekday  int
	millis   int64
	raw, dst int64
}

func (f *DateTimeFormat) number(value int64, minDigits int) string {
	return f.numberAffixed(f.digits, f.digitPrefix, value, minDigits)
}

func (f *DateTimeFormat) numberIn(digits []rune, value int64, minDigits int) string {
	return f.numberAffixed(digits, "", value, minDigits)
}

// numberAffixed writes a field the way the formatter's NumberFormat does; prefix is the literal text of the locale's
// decimal pattern for its numbering system (az with arab and arabext has one, and it shows on every field).
func (f *DateTimeFormat) numberAffixed(digits []rune, prefix string, value int64, minDigits int) string {
	neg := value < 0
	if neg {
		value = -value
	}
	text := strconv.FormatInt(value, 10)
	if len(text) < minDigits {
		text = strings.Repeat("0", minDigits-len(text)) + text
	}
	var b strings.Builder
	if neg {
		b.WriteString(f.minusSign())
	}
	b.WriteString(prefix)
	for i := 0; i < len(text); i++ {
		if len(digits) == 10 {
			b.WriteRune(digits[text[i]-'0'])
		} else {
			b.WriteByte(text[i])
		}
	}
	return b.String()
}

func (f *DateTimeFormat) minusSign() string {
	if s := lookupString("main", f.locale, "NumberElements", f.ns, "symbols", "minusSign"); s != "" {
		return s
	}
	return lookupString("main", f.locale, "NumberElements", "latn", "symbols", "minusSign")
}

// calendarNames looks a field up in the calendar's data, then in the Gregorian data.
func (f *DateTimeFormat) calendarNames(path ...string) any {
	full := append([]string{"calendar", f.resource}, path...)
	if v := lookup("main", f.locale, full...); v != nil {
		return v
	}
	full[1] = "gregorian"
	return lookup("main", f.locale, full...)
}

func widthName(count int) string {
	switch {
	case count == 4:
		return "wide"
	case count == 5:
		return "narrow"
	case count == 6:
		return "short"
	}
	return "abbreviated"
}

func (f *DateTimeFormat) nameAt(table string, context, width string, index int) string {
	list, _ := f.calendarNames(table, context, width).([]any)
	if index < len(list) {
		s, _ := list[index].(string)
		return s
	}
	return ""
}

func (f *DateTimeFormat) monthField(letter byte, count int, c *dateContext) string {
	value := c.fields.month
	if f.calendar == "hebrew" {
		if c.fields.leap && value == 6 && count >= 3 {
			value = 13
		}
		if !c.fields.leap && value >= 6 && count < 3 {
			value--
		}
	}
	if count < 3 {
		if f.numbers == "M=romanlow" {
			return romanLower[value]
		}
		return f.number(int64(value+1), count)
	}
	context := "format"
	if letter == 'L' {
		context = "stand-alone"
	}
	return f.nameAt("monthNames", context, widthName(count), value)
}

func (f *DateTimeFormat) weekdayField(letter byte, count int, c *dateContext) string {
	if letter != 'E' && count < 3 {
		first := 1
		return f.number(floorMod(int64(c.weekday-first), 7)+1, count)
	}
	context := "format"
	if letter == 'c' {
		context = "stand-alone"
	}
	width := widthName(count)
	return f.nameAt("dayNames", context, width, c.weekday-1)
}

func (f *DateTimeFormat) eraField(count int, c *dateContext) string {
	if f.calendar == "iso8601" && c.fields.era != 0 {
		return ""
	}
	width := widthName(count)
	if count < 4 {
		width = "abbreviated"
	}
	key := strconv.Itoa(c.fields.era)
	if lookup("main", f.locale, "calendar", f.resource, "eras", width) != nil {
		s := lookupString("main", f.locale, "calendar", f.resource, "eras", width, key)
		return s
	}
	s, _ := f.calendarNames("eras", width, key).(string)
	return s
}

func (f *DateTimeFormat) ampmText(c *dateContext, count int) string {
	hour := c.millis / 3600000
	idx := 0
	if hour >= 12 {
		idx = 1
	}
	key := "AmPmMarkersAbbr"
	switch {
	case count == 4:
		key = "AmPmMarkers"
	case count == 5:
		key = "AmPmMarkersNarrow"
	}
	list := toStrings(f.calendarNames(key))
	if idx < len(list) {
		return list[idx]
	}
	return ""
}

// dayPeriodRuleSet returns the flexible day periods of the locale, nil when it has none.
func dayPeriodRuleSet(locale string) map[string]any {
	data := icudata.Table("supp/dayPeriods")
	locales, _ := data["locales"].(map[string]any)
	rules, _ := data["rules"].(map[string]any)
	name := locale
	for name != "" {
		if set, ok := locales[name].(string); ok {
			t, _ := rules[set].(map[string]any)
			return t
		}
		name = chop(name)
	}
	return nil
}

func hourOfSpec(s string) int {
	h, _, _ := strings.Cut(s, ":")
	n, _ := strconv.Atoi(h)
	return n
}

func (f *DateTimeFormat) flexiblePeriod(c *dateContext, count int) string {
	rules := dayPeriodRuleSet(f.locale)
	if rules == nil {
		return f.ampmText(c, count)
	}
	hour := int(c.millis / 3600000)
	minute, second := 0, 0
	if f.hasMin {
		minute = int(c.millis / 60000 % 60)
	}
	if f.hasSec {
		second = int(c.millis / 1000 % 60)
	}
	period := ""
	if _, ok := rules["noon"]; ok && hour == 12 && minute == 0 && second == 0 {
		period = "noon"
	}
	if period == "" {
		for name, v := range rules {
			t, ok := v.(map[string]any)
			if !ok {
				continue
			}
			from, hasFrom := t["from"].(string)
			before, hasBefore := t["before"].(string)
			if !hasFrom || !hasBefore {
				continue
			}
			a, b := hourOfSpec(from), hourOfSpec(before)
			if a < b && hour >= a && hour < b || a >= b && (hour >= a || hour < b) {
				period = name
				break
			}
		}
	}
	if period == "" {
		return f.ampmText(c, count)
	}
	width := widthName(count)
	if count < 4 {
		width = "abbreviated"
	}
	for _, path := range [][]string{{"dayPeriod", "format", width, period}, {"dayPeriod", "format", "abbreviated", period}} {
		if s, _ := f.calendarNames(path...).(string); s != "" {
			return s
		}
	}
	return f.ampmText(c, count)
}

func (f *DateTimeFormat) field(letter byte, count int, c *dateContext) string {
	hour := c.millis / 3600000
	switch letter {
	case 'G':
		return f.eraField(count, c)
	case 'y':
		y := c.fields.year
		switch f.numbers {
		case "hebr":
			return hebrewNumeral(y)
		case "y=jpanyear":
			if y == 1 {
				return "元"
			}
		}
		if count == 2 {
			return f.twoDigits(y)
		}
		return f.number(y, count)
	case 'Y':
		y := f.weekYear(c)
		if count == 2 {
			return f.twoDigits(y)
		}
		return f.number(y, count)
	case 'M', 'L':
		return f.monthField(letter, count, c)
	case 'E', 'c', 'e':
		return f.weekdayField(letter, count, c)
	case 'd':
		if f.numbers == "hebr" {
			return hebrewNumber(c.fields.day)
		}
		return f.number(c.fields.day, count)
	case 'a':
		return f.ampmText(c, count)
	case 'b':
		return f.nearestNoonText(c, count)
	case 'B':
		return f.flexiblePeriod(c, count)
	case 'H':
		return f.number(hour, count)
	case 'k':
		if hour == 0 {
			return f.number(24, count)
		}
		return f.number(hour, count)
	case 'h':
		h := hour % 12
		if h == 0 {
			h = 12
		}
		return f.number(h, count)
	case 'K':
		return f.number(hour%12, count)
	case 'm':
		return f.number(c.millis/60000%60, count)
	case 's':
		return f.number(c.millis/1000%60, count)
	case 'S':
		ms := c.millis % 1000
		text := strconv.FormatInt(ms+1000, 10)[1:]
		if count > 3 {
			text += strings.Repeat("0", count-3)
		}
		return f.digitsOf(text[:count])
	case 'z', 'v', 'O', 'V':
		return f.zoneText(letter, count, c)
	}
	return ""
}

// twoDigits writes a year's last two digits; ICU keeps the sign of a negative year.
func (f *DateTimeFormat) twoDigits(y int64) string {
	if y < 0 {
		return f.minusSign() + f.number(-y%100, 2)
	}
	return f.number(y%100, 2)
}

// weekYear is the year of the week of the year, from the locale's first day of the week and minimal days.
func (f *DateTimeFormat) weekYear(c *dateContext) int64 {
	switch f.calendar {
	case "gregory", "iso8601", "buddhist", "roc", "japanese":
	default:
		return c.fields.year
	}
	region := f.targetRegion()
	weeks, _ := icudata.Table("supp/time")["weekData"].(map[string]any)
	data, ok := weeks[region].([]any)
	if !ok {
		data, _ = weeks["001"].([]any)
	}
	first, _ := data[0].(float64)
	minDays, _ := data[1].(float64)
	day := floorDiv(c.t+c.raw+c.dst, 86400000)
	year, _, _ := civilFromDays(day)
	yearStart := func(y int64) int64 { return daysFromCivil(y, 1, 1) }
	if f.calendar != "gregory" && f.calendar != "iso8601" {
		// These run on GregorianCalendar with its 1582 cutover; V8 makes only gregory and iso8601 proleptic.
		year, _, _ = gregorianJulianFields(day + epochJD)
		yearStart = func(y int64) int64 {
			if y > 1582 {
				return daysFromCivil(y, 1, 1)
			}
			return 1721424 + 365*(y-1) + floorDiv(y-1, 4) - epochJD
		}
	}
	yearLength := func(y int64) int64 { return yearStart(y+1) - yearStart(y) }
	doy := day - yearStart(year)
	dow := int64(c.weekday)
	relDow := floorMod(dow+7-int64(first), 7)
	relJan1 := floorMod(dow-doy+7001-int64(first), 7)
	week := (doy - relDow + relJan1) / 7
	if 7-relJan1 >= int64(minDays) {
		week++
	}
	if week == 0 {
		return year - 1
	}
	last := yearLength(year) - 1
	if doy >= last-5 {
		lastRel := floorMod(relDow+last-doy, 7)
		if 6-lastRel >= int64(minDays) && doy+7-relDow > last {
			return year + 1
		}
	}
	return year
}

var romanLower = []string{"i", "ii", "iii", "iv", "v", "vi", "vii", "viii", "ix", "x", "xi", "xii", "xiii"}

func (f *DateTimeFormat) digitsOf(text string) string {
	var b strings.Builder
	b.WriteString(f.digitPrefix)
	for i := 0; i < len(text); i++ {
		if len(f.digits) == 10 {
			b.WriteRune(f.digits[text[i]-'0'])
		} else {
			b.WriteByte(text[i])
		}
	}
	return b.String()
}

func (f *DateTimeFormat) nearestNoonText(c *dateContext, count int) string {
	if c.millis == 12*3600000 {
		key := "NoonMarker"
		if count == 5 {
			key = "NoonMarkerNarrow"
		}
		if s, _ := f.calendarNames(key).(string); s != "" {
			return s
		}
	}
	return f.ampmText(c, count)
}
