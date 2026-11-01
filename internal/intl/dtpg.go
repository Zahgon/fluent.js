package intl

import (
	"strconv"
	"strings"
)

// Field numbers of ICU's DateTimePatternGenerator.
const (
	fieldEra = iota
	fieldYear
	fieldQuarter
	fieldMonth
	fieldWeekOfYear
	fieldWeekOfMonth
	fieldWeekday
	fieldDay
	fieldDayOfYear
	fieldDayOfWeekInMonth
	fieldDayPeriod
	fieldHour
	fieldMinute
	fieldSecond
	fieldFractionalSecond
	fieldZone
	fieldCount
)

const (
	dtNarrow  = -0x101
	dtShorter = -0x102
	dtShort   = -0x103
	dtLong    = -0x104
	dtNumeric = 0x100
	dtDelta   = 0x10

	dateMask = (1 << fieldDayPeriod) - 1
	timeMask = (1 << fieldCount) - 1 - dateMask

	distanceMissing = 0x1000
	distanceExtra   = 0x10000
)

// dtType is a row of ICU's dtTypes table.
type dtType struct {
	char   byte
	field  int
	typ    int
	minLen int
}

var dtTypes = []dtType{
	{'G', fieldEra, dtShort, 1}, {'G', fieldEra, dtLong, 4}, {'G', fieldEra, dtNarrow, 5},

	{'y', fieldYear, dtNumeric, 1}, {'Y', fieldYear, dtNumeric + dtDelta, 1}, {'u', fieldYear, dtNumeric + 2*dtDelta, 1},
	{'r', fieldYear, dtNumeric + 3*dtDelta, 1}, {'U', fieldYear, dtShort, 1}, {'U', fieldYear, dtLong, 4}, {'U', fieldYear, dtNarrow, 5},

	{'Q', fieldQuarter, dtNumeric, 1}, {'Q', fieldQuarter, dtShort, 3}, {'Q', fieldQuarter, dtLong, 4}, {'Q', fieldQuarter, dtNarrow, 5},
	{'q', fieldQuarter, dtNumeric + dtDelta, 1}, {'q', fieldQuarter, dtShort - dtDelta, 3}, {'q', fieldQuarter, dtLong - dtDelta, 4}, {'q', fieldQuarter, dtNarrow - dtDelta, 5},

	{'M', fieldMonth, dtNumeric, 1}, {'M', fieldMonth, dtShort, 3}, {'M', fieldMonth, dtLong, 4}, {'M', fieldMonth, dtNarrow, 5},
	{'L', fieldMonth, dtNumeric + dtDelta, 1}, {'L', fieldMonth, dtShort - dtDelta, 3}, {'L', fieldMonth, dtLong - dtDelta, 4}, {'L', fieldMonth, dtNarrow - dtDelta, 5},
	{'l', fieldMonth, dtNumeric + 2*dtDelta, 1},

	{'w', fieldWeekOfYear, dtNumeric, 1},
	{'W', fieldWeekOfMonth, dtNumeric, 1},

	{'E', fieldWeekday, dtShort, 1}, {'E', fieldWeekday, dtLong, 4}, {'E', fieldWeekday, dtNarrow, 5}, {'E', fieldWeekday, dtShorter, 6},
	{'c', fieldWeekday, dtNumeric + 2*dtDelta, 1}, {'c', fieldWeekday, dtShort - 2*dtDelta, 3}, {'c', fieldWeekday, dtLong - 2*dtDelta, 4}, {'c', fieldWeekday, dtNarrow - 2*dtDelta, 5}, {'c', fieldWeekday, dtShorter - 2*dtDelta, 6},
	{'e', fieldWeekday, dtNumeric + dtDelta, 1}, {'e', fieldWeekday, dtShort - dtDelta, 3}, {'e', fieldWeekday, dtLong - dtDelta, 4}, {'e', fieldWeekday, dtNarrow - dtDelta, 5}, {'e', fieldWeekday, dtShorter - dtDelta, 6},

	{'d', fieldDay, dtNumeric, 1}, {'g', fieldDay, dtNumeric + dtDelta, 1},
	{'D', fieldDayOfYear, dtNumeric, 1},
	{'F', fieldDayOfWeekInMonth, dtNumeric, 1},

	{'a', fieldDayPeriod, dtShort, 1}, {'a', fieldDayPeriod, dtLong, 4}, {'a', fieldDayPeriod, dtNarrow, 5},
	{'b', fieldDayPeriod, dtShort - dtDelta, 1}, {'b', fieldDayPeriod, dtLong - dtDelta, 4}, {'b', fieldDayPeriod, dtNarrow - dtDelta, 5},
	{'B', fieldDayPeriod, dtShort - 3*dtDelta, 1}, {'B', fieldDayPeriod, dtLong - 3*dtDelta, 4}, {'B', fieldDayPeriod, dtNarrow - 3*dtDelta, 5},

	{'H', fieldHour, dtNumeric + 10*dtDelta, 1}, {'k', fieldHour, dtNumeric + 11*dtDelta, 1},
	{'h', fieldHour, dtNumeric, 1}, {'K', fieldHour, dtNumeric + dtDelta, 1},

	{'m', fieldMinute, dtNumeric, 1},
	{'s', fieldSecond, dtNumeric, 1}, {'A', fieldSecond, dtNumeric + dtDelta, 1},
	{'S', fieldFractionalSecond, dtNumeric, 1},

	{'v', fieldZone, dtShort - 2*dtDelta, 1}, {'v', fieldZone, dtLong - 2*dtDelta, 4},
	{'z', fieldZone, dtShort, 1}, {'z', fieldZone, dtLong, 4},
	{'Z', fieldZone, dtNarrow - dtDelta, 1}, {'Z', fieldZone, dtLong - dtDelta, 4}, {'Z', fieldZone, dtShort - dtDelta, 5},
	{'O', fieldZone, dtShort - dtDelta, 1}, {'O', fieldZone, dtLong - dtDelta, 4},
	{'V', fieldZone, dtShort - dtDelta, 1}, {'V', fieldZone, dtLong - dtDelta, 2}, {'V', fieldZone, dtLong - 1 - dtDelta, 3}, {'V', fieldZone, dtLong - 2 - dtDelta, 4},
	{'X', fieldZone, dtNarrow - dtDelta, 1}, {'X', fieldZone, dtShort - dtDelta, 2}, {'X', fieldZone, dtLong - dtDelta, 4},
	{'x', fieldZone, dtNarrow - dtDelta, 1}, {'x', fieldZone, dtShort - dtDelta, 2}, {'x', fieldZone, dtLong - dtDelta, 4},
}

// canonicalIndex is ICU's FormatParser::getCanonicalIndex with strict=true: the row for a run of one letter, or -1.
func canonicalIndex(item string) int {
	if item == "" {
		return -1
	}
	ch := item[0]
	for i := 1; i < len(item); i++ {
		if item[i] != ch {
			return -1
		}
	}
	for i := 0; i < len(dtTypes); i++ {
		if dtTypes[i].char != ch {
			continue
		}
		if i+1 >= len(dtTypes) || dtTypes[i+1].char != ch {
			return i
		}
		if dtTypes[i+1].minLen <= len(item) {
			continue
		}
		return i
	}
	return -1
}

// patternItems splits a pattern as ICU's FormatParser does: runs of one ASCII letter, quoted literals with their
// quotes, and every other character alone.
func patternItems(p string) []string {
	var items []string
	runes := []rune(p)
	for i := 0; i < len(runes); {
		c := runes[i]
		switch {
		case c < 0x80 && (c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'):
			j := i + 1
			for j < len(runes) && runes[j] == c {
				j++
			}
			items = append(items, string(runes[i:j]))
			i = j
		case c == '\'':
			j := i + 1
			for j < len(runes) {
				if runes[j] == '\'' {
					if j+1 < len(runes) && runes[j+1] == '\'' {
						j += 2
						continue
					}
					break
				}
				j++
			}
			if j < len(runes) {
				j++
			}
			items = append(items, string(runes[i:j]))
			i = j
		default:
			items = append(items, string(c))
			i++
		}
	}
	return items
}

// ptnSkeleton is ICU's PtnSkeleton.
type ptnSkeleton struct {
	typ          [fieldCount]int
	original     [fieldCount]string
	baseOriginal [fieldCount]string
}

func parseSkeleton(pattern string) ptnSkeleton {
	var sk ptnSkeleton
	for _, item := range patternItems(pattern) {
		if strings.HasPrefix(item, "'") {
			continue
		}
		idx := canonicalIndex(item)
		if idx < 0 {
			continue
		}
		row := dtTypes[idx]
		sk.original[row.field] = item
		sk.baseOriginal[row.field] = strings.Repeat(string(row.char), row.minLen)
		sub := row.typ
		if row.typ > 0 {
			sub += len(item)
		}
		sk.typ[row.field] = sub
	}
	if sk.original[fieldMinute] != "" && sk.original[fieldFractionalSecond] != "" && sk.original[fieldSecond] == "" {
		sk.original[fieldSecond] = "s"
		sk.baseOriginal[fieldSecond] = "s"
		sk.typ[fieldSecond] = dtNumeric + 1
	}
	if h := sk.original[fieldHour]; h != "" {
		if h[0] == 'h' || h[0] == 'K' {
			if sk.original[fieldDayPeriod] == "" {
				sk.original[fieldDayPeriod] = "a"
				sk.baseOriginal[fieldDayPeriod] = "a"
				sk.typ[fieldDayPeriod] = dtShort
			}
		} else {
			sk.original[fieldDayPeriod] = ""
			sk.baseOriginal[fieldDayPeriod] = ""
			sk.typ[fieldDayPeriod] = 0
		}
	}
	return sk
}

func (s *ptnSkeleton) base() string     { return strings.Join(s.baseOriginal[:], "") }
func (s *ptnSkeleton) skeleton() string { return strings.Join(s.original[:], "") }

func (s *ptnSkeleton) fieldMask() int {
	mask := 0
	for i, t := range s.typ {
		if t != 0 {
			mask |= 1 << i
		}
	}
	return mask
}

// ptnElem is an entry of ICU's PatternMap.
type ptnElem struct {
	base      string
	skeleton  ptnSkeleton
	pattern   string
	specified bool
}

// patternGenerator is ICU's DateTimePatternGenerator for one locale and calendar.
type patternGenerator struct {
	ns          string
	locale      string
	calendar    string
	boot        [52][]*ptnElem
	appendItems [fieldCount]string
	fieldNames  [fieldCount]string
	dateTime    [4]string
	hourChar    byte
	matcher     ptnSkeleton
	matchHour   bool
}

func baseIndex(c byte) int {
	if c >= 'A' && c <= 'Z' {
		return int(c - 'A')
	}
	return 26 + int(c-'a')
}

func (g *patternGenerator) add(base string, sk ptnSkeleton, pattern string, specified bool) {
	list := g.boot[baseIndex(base[0])]
	for _, e := range list {
		if e.base == base && e.skeleton.skeleton() == sk.skeleton() {
			e.pattern = pattern
			e.specified = specified
			return
		}
	}
	g.boot[baseIndex(base[0])] = append(list, &ptnElem{base, sk, pattern, specified})
}

func (g *patternGenerator) fromBase(base string) *ptnElem {
	if base == "" {
		return nil
	}
	for _, e := range g.boot[baseIndex(base[0])] {
		if e.base == base {
			return e
		}
	}
	return nil
}

func (g *patternGenerator) fromSkeleton(sk ptnSkeleton) *ptnElem {
	base := sk.base()
	if base == "" {
		return nil
	}
	want := sk.skeleton()
	for _, e := range g.boot[baseIndex(base[0])] {
		if e.skeleton.skeleton() == want {
			return e
		}
	}
	return nil
}

// addPattern is addPatternWithSkeleton.
func (g *patternGenerator) addPattern(pattern, skeletonToUse string, override, hasSkeleton bool) {
	var sk ptnSkeleton
	if hasSkeleton {
		sk = parseSkeleton(skeletonToUse)
	} else {
		sk = parseSkeleton(pattern)
	}
	base := sk.base()
	if base == "" {
		return
	}
	if dup := g.fromBase(base); dup != nil && (!dup.specified || hasSkeleton && !override) {
		if !override {
			return
		}
	}
	if dup := g.fromSkeleton(sk); dup != nil {
		if !override || hasSkeleton && dup.specified {
			return
		}
	}
	g.add(base, sk, pattern, hasSkeleton)
}

var appendKeys = map[string]int{"Era": fieldEra, "Year": fieldYear, "Quarter": fieldQuarter, "Month": fieldMonth, "Week": fieldWeekOfYear,
	"Day-Of-Week": fieldWeekday, "Day": fieldDay, "Hour": fieldHour, "Minute": fieldMinute, "Second": fieldSecond, "Timezone": fieldZone}

var nameKeys = map[string]int{"era": fieldEra, "year": fieldYear, "quarter": fieldQuarter, "month": fieldMonth, "week": fieldWeekOfYear,
	"weekOfMonth": fieldWeekOfMonth, "weekday": fieldWeekday, "day": fieldDay, "dayOfYear": fieldDayOfYear, "weekdayOfMonth": fieldDayOfWeekInMonth,
	"dayperiod": fieldDayPeriod, "hour": fieldHour, "minute": fieldMinute, "second": fieldSecond, "zone": fieldZone}

var generatorCache = map[string]*patternGenerator{}

func newPatternGenerator(locale, calendar, ns string, hourChar byte) *patternGenerator {
	return newPatternGeneratorOptions(locale, calendar, ns, hourChar, false)
}

// newPatternGeneratorOptions builds a generator, with createInstanceNoStdPat's skipping of the standard patterns.
func newPatternGeneratorOptions(locale, calendar, ns string, hourChar byte, noStd bool) *patternGenerator {
	key := locale + "/" + calendar + "/" + ns + "/" + string(hourChar) + "/" + strconv.FormatBool(noStd)
	if g, ok := generatorCache[key]; ok {
		return g
	}
	g := &patternGenerator{locale: locale, calendar: calendar, ns: ns, hourChar: hourChar}
	for _, item := range []string{"G", "y", "Q", "M", "w", "W", "E", "d", "D", "F", "a", "H", "m", "s", "S", "v"} {
		g.addPattern(item, "", false, false)
	}
	patterns, _ := lookup("main", locale, "calendar", calendar, "DateTimePatterns").([]any)
	for i := 0; !noStd && i < 8 && i < len(patterns); i++ {
		g.addPattern(patternText(patterns[i]), "", false, false)
	}
	for k, v := range merged("main", locale, "calendar", calendar, "appendItems") {
		if f, ok := appendKeys[k]; ok {
			g.appendItems[f], _ = v.(string)
		}
	}
	for k, f := range nameKeys {
		if name := lookupString("main", locale, "fields", k, "dn"); name != "" {
			g.fieldNames[f] = name
		}
	}
	for f := range g.appendItems {
		if g.appendItems[f] == "" {
			g.appendItems[f] = "{0} ├{2}: {1}┤"
		}
		if g.fieldNames[f] == "" {
			g.fieldNames[f] = "F" + strconv.Itoa(f)
		}
	}
	g.addAvailableFormats()
	g.setDateTimeFormats()
	generatorCache[key] = g
	return g
}

func patternText(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []any:
		if len(x) > 0 {
			s, _ := x[0].(string)
			return s
		}
	}
	return ""
}

// addAvailableFormats walks availableFormats over the chain, a more specific bundle first, following aliases as ICU's
// getAllChildrenWithFallback does.
func (g *patternGenerator) addAvailableFormats() {
	set := map[string]bool{}
	path := []string{"calendar", g.calendar, "availableFormats"}
	for depth := 0; depth < 8; depth++ {
		var redirect []string
		for _, name := range chain("main", g.locale) {
			r := walkResultFor("main", name, path)
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
				if set[key] {
					continue
				}
				set[key] = true
				text, _ := table[key].(string)
				g.addPattern(text, key, true, true)
			}
		}
		if redirect == nil {
			return
		}
		path = redirect
	}
}

func (g *patternGenerator) setDateTimeFormats() {
	var list []string
	if g.calendar != "gregorian" {
		list = lookupStrings("main", g.locale, "calendar", g.calendar, "DateTimePatterns%atTime")
	}
	if len(list) < 4 {
		list = lookupStrings("main", g.locale, "calendar", "gregorian", "DateTimePatterns%atTime")
	}
	if len(list) >= 4 {
		copy(g.dateTime[:], list[:4])
		return
	}
	patterns, _ := lookup("main", g.locale, "calendar", g.calendar, "DateTimePatterns").([]any)
	for i := 0; i < 4 && i+9 < len(patterns); i++ {
		g.dateTime[i] = patternText(patterns[i+9])
	}
}

type distanceInfo struct{ missing, extra int }

func skeletonDistance(a, b *ptnSkeleton, include int) (int, distanceInfo) {
	result := 0
	var info distanceInfo
	for i := 0; i < fieldCount; i++ {
		my := 0
		if include&(1<<i) != 0 {
			my = a.typ[i]
		}
		other := b.typ[i]
		if my == other {
			continue
		}
		switch {
		case my == 0:
			result += distanceExtra
			info.extra |= 1 << i
		case other == 0:
			result += distanceMissing
			info.missing |= 1 << i
		default:
			if my > other {
				result += my - other
			} else {
				result += other - my
			}
		}
	}
	return result, info
}

func (g *patternGenerator) bestRaw(include int) (*ptnElem, distanceInfo) {
	best := 0x7fffffff
	bestMissing := -1
	var bestElem *ptnElem
	var bestInfo distanceInfo
	for i := range g.boot {
		for _, e := range g.boot[i] {
			d, info := skeletonDistance(&g.matcher, &e.skeleton, include)
			if d < best || d == best && bestMissing < info.missing {
				best = d
				bestMissing = info.missing
				bestElem = e
				bestInfo = info
				if d == 0 {
					return bestElem, bestInfo
				}
			}
		}
	}
	return bestElem, bestInfo
}

const fixFractionalSeconds = 1

func (g *patternGenerator) adjust(pattern string, specified *ptnSkeleton, flags int) string {
	var out strings.Builder
	for _, item := range patternItems(pattern) {
		if strings.HasPrefix(item, "'") {
			out.WriteString(item)
			continue
		}
		idx := canonicalIndex(item)
		if idx < 0 {
			out.WriteString(item)
			continue
		}
		row := dtTypes[idx]
		field := row.field
		switch {
		case flags&fixFractionalSeconds != 0 && field == fieldSecond:
			out.WriteString(item)
			out.WriteString(g.decimalSymbol())
			out.WriteString(g.matcher.original[fieldFractionalSecond])
			continue
		case g.matcher.typ[field] != 0:
			req := g.matcher.original[field]
			reqChar := req[0]
			reqLen := len(req)
			if reqChar == 'E' && reqLen < 3 {
				reqLen = 3
			}
			adj := reqLen
			switch {
			case field == fieldHour && !g.matchHour || field == fieldMinute || field == fieldSecond:
				adj = len(item)
			case specified != nil && reqChar != 'c' && reqChar != 'e':
				skelLen := len(specified.original[field])
				patNumeric := row.typ > 0
				reqNumeric := g.matcher.typ[field] > 0
				if skelLen == reqLen || patNumeric != reqNumeric {
					adj = len(item)
				}
			}
			c := item[0]
			if field != fieldHour && field != fieldMonth && field != fieldWeekday && (field != fieldYear || reqChar == 'Y') {
				c = reqChar
			}
			if c == 'E' && adj < 3 {
				c = 'e'
			}
			if field == fieldHour && g.hourChar != 0 {
				switch {
				case reqChar == g.hourChar:
					c = g.hourChar
				case reqChar == 'h' && g.hourChar == 'K':
					c = 'K'
				case reqChar == 'H' && g.hourChar == 'k':
					c = 'k'
				case reqChar == 'k' && g.hourChar == 'H':
					c = 'H'
				case reqChar == 'K' && g.hourChar == 'h':
					c = 'h'
				}
			}
			out.WriteString(strings.Repeat(string(c), adj))
			continue
		}
		out.WriteString(item)
	}
	return out.String()
}

// decimalSymbol is DecimalFormatSymbols' decimal for the formatter's numbering system, then the locale's Latin one.
func (g *patternGenerator) decimalSymbol() string {
	if s := lookupString("main", g.locale, "NumberElements", g.ns, "symbols", "decimal"); s != "" {
		return s
	}
	return lookupString("main", g.locale, "NumberElements", "latn", "symbols", "decimal")
}

func (g *patternGenerator) specifiedOf(e *ptnElem) *ptnSkeleton {
	if e.specified {
		return &e.skeleton
	}
	return nil
}

func topBit(mask int) int {
	i := 0
	for mask != 0 {
		mask >>= 1
		i++
	}
	if i-1 > fieldZone {
		return fieldZone
	}
	return i - 1
}

func (g *patternGenerator) bestAppending(missing int) string {
	if missing == 0 {
		return ""
	}
	e, info := g.bestRaw(missing)
	result := g.adjust(e.pattern, g.specifiedOf(e), 0)
	last := 0
	for info.missing != 0 {
		if last == info.missing {
			break
		}
		if info.missing&(1<<fieldSecond|1<<fieldFractionalSecond) == 1<<fieldFractionalSecond &&
			missing&(1<<fieldSecond|1<<fieldFractionalSecond) == 1<<fieldSecond|1<<fieldFractionalSecond {
			result = g.adjust(result, g.specifiedOf(e), fixFractionalSeconds)
			info.missing &^= 1 << fieldFractionalSecond
			continue
		}
		start := info.missing
		next, nextInfo := g.bestRaw(info.missing)
		temp := g.adjust(next.pattern, g.specifiedOf(next), 0)
		found := start &^ nextInfo.missing
		top := topBit(found)
		format := g.appendItems[top]
		name := "'" + g.fieldNames[top] + "'"
		result = strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(format, "{0}", result), "{1}", temp), "{2}", name)
		e, info = next, nextInfo
		last = info.missing
	}
	return result
}

// bestPattern is getBestPattern(skeleton, UDATPG_MATCH_HOUR_FIELD_LENGTH).
func (g *patternGenerator) bestPattern(skeleton string) string {
	return g.bestPatternOptions(skeleton, true)
}

// bestPatternOptions is getBestPattern with or without UDATPG_MATCH_HOUR_FIELD_LENGTH.
func (g *patternGenerator) bestPatternOptions(skeleton string, matchHour bool) string {
	g.matchHour = matchHour
	g.matcher = parseSkeleton(skeleton)
	e, info := g.bestRaw(-1)
	if info.missing == 0 && info.extra == 0 {
		return g.adjust(e.pattern, g.specifiedOf(e), 0)
	}
	needed := g.matcher.fieldMask()
	date := g.bestAppending(needed & dateMask)
	clock := g.bestAppending(needed & timeMask)
	if date == "" {
		return clock
	}
	if clock == "" {
		return date
	}
	style := 3
	switch len(g.matcher.baseOriginal[fieldMonth]) {
	case 4:
		style = 1
		if len(g.matcher.baseOriginal[fieldWeekday]) > 0 {
			style = 0
		}
	case 3:
		style = 2
	}
	return strings.ReplaceAll(strings.ReplaceAll(g.dateTime[style], "{0}", clock), "{1}", date)
}
