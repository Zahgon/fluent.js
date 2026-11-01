package intl

import (
	"encoding/hex"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/projectfluent/fluent-go/internal/icudata"
)

// zoneRule is the SimpleTimeZone a zone follows after its last explicit transition.
type zoneRule struct {
	startMonth, startDay, startDOW, startTime, startMode int
	endMonth, endDay, endDOW, endTime, endMode           int
	savings                                              int
}

// timeZone is ICU's OlsonTimeZone, or a fixed offset zone when custom is set.
type timeZone struct {
	id          string
	custom      bool
	fixedOffset int64 // milliseconds
	trans       []int64
	typeOffsets []int64
	typeMap     []byte
	finalRule   *zoneRule
	finalRaw    int64 // seconds
	finalYear   int64
}

func zoneTables() (names []any, zones []any, rules map[string]any, regions []any) {
	data := icudata.Table("supp/zoneinfo")
	names, _ = data["Names"].([]any)
	zones, _ = data["Zones"].([]any)
	rules, _ = data["Rules"].(map[string]any)
	regions, _ = data["Regions"].([]any)
	return
}

var zoneIndexCache map[string]int
var zoneLowerCache map[string]string

func zoneIndex() {
	if zoneIndexCache != nil {
		return
	}
	names, _, _, _ := zoneTables()
	zoneIndexCache = make(map[string]int, len(names))
	zoneLowerCache = make(map[string]string, len(names))
	for i, n := range names {
		s := n.(string)
		zoneIndexCache[s] = i
		zoneLowerCache[strings.ToLower(s)] = s
	}
}

func int64List(v any) []int64 {
	list, _ := v.([]any)
	out := make([]int64, len(list))
	for i, x := range list {
		out[i] = int64(x.(float64))
	}
	return out
}

var zoneCache = map[string]*timeZone{}

// loadZone returns the zone for an exact id in zoneinfo, nil when there is none.
func loadZone(id string) *timeZone {
	if z, ok := zoneCache[id]; ok {
		return z
	}
	zoneIndex()
	idx, ok := zoneIndexCache[id]
	if !ok {
		zoneCache[id] = nil
		return nil
	}
	_, zones, rules, _ := zoneTables()
	entry := zones[idx]
	for {
		n, isLink := entry.(float64)
		if !isLink {
			break
		}
		entry = zones[int(n)]
	}
	t := entry.(map[string]any)
	z := &timeZone{id: id}
	pre := int64List(t["transPre32"])
	for i := 0; i+1 < len(pre); i += 2 {
		z.trans = append(z.trans, pre[i]<<32|(pre[i+1]&0xffffffff))
	}
	z.trans = append(z.trans, int64List(t["trans"])...)
	post := int64List(t["transPost32"])
	for i := 0; i+1 < len(post); i += 2 {
		z.trans = append(z.trans, post[i]<<32|(post[i+1]&0xffffffff))
	}
	z.typeOffsets = int64List(t["typeOffsets"])
	if tm, ok := t["typeMap"].(map[string]any); ok {
		z.typeMap, _ = hex.DecodeString(tm["$binary"].(string))
	}
	if name, ok := t["finalRule"].(string); ok {
		r := int64List(rules[name])
		z.finalRule = &zoneRule{int(r[0]), int(r[1]), int(r[2]), int(r[3]) * 1000, int(r[4]), int(r[5]), int(r[6]), int(r[7]), int(r[8]) * 1000, int(r[9]), int(r[10]) * 1000}
		z.finalRaw = int64(t["finalRaw"].(float64))
		z.finalYear = int64(t["finalYear"].(float64))
	}
	zoneCache[id] = z
	return z
}

// offsetZone parses the time zone forms +HH, +HHMM, +HH:MM; ok is false for anything else.
func offsetZone(s string) (*timeZone, bool) {
	if len(s) < 3 || s[0] != '+' && s[0] != '-' && !strings.HasPrefix(s, "−") {
		return nil, false
	}
	sign := int64(1)
	if s[0] == '-' {
		sign = -1
	}
	rest := s[1:]
	var h, m string
	switch {
	case len(rest) == 2:
		h, m = rest, "00"
	case len(rest) == 4:
		h, m = rest[:2], rest[2:]
	case len(rest) == 5 && rest[2] == ':':
		h, m = rest[:2], rest[3:]
	default:
		return nil, false
	}
	if !isASCIIDigits(h) || !isASCIIDigits(m) {
		return nil, false
	}
	hh, _ := strconv.Atoi(h)
	mm, _ := strconv.Atoi(m)
	if hh > 23 || mm > 59 {
		return nil, false
	}
	if hh == 0 && mm == 0 {
		// ICU resolves a zero offset to its GMT zone, names included; only the literal names GMT and Etc/GMT become UTC.
		return loadZone("Etc/GMT"), true
	}
	id := s[:1] + h + ":" + m
	return &timeZone{id: id, custom: true, fixedOffset: sign * int64(hh*3600000+mm*60000)}, true
}

func resolveTimeZone(name string) *timeZone {
	if z, ok := offsetZone(name); ok {
		return z
	}
	zoneIndex()
	lower := strings.ToLower(name)
	switch lower {
	case "utc", "etc/utc", "etc/gmt", "gmt":
		lower = "utc"
	}
	id, ok := zoneLowerCache[lower]
	if !ok || lower == "factory" {
		rangeError("Invalid time zone specified: " + name)
	}
	return loadZone(id)
}

func (r *zoneRule) dayFor(year int64, month, day, dow int) int64 {
	m := int64(month + 1)
	first := daysFromCivil(year, m, 1)
	length := int(daysFromCivil(year, m+1, 1) - first)
	if m == 12 {
		length = 31
	}
	weekday := func(d int64) int { return int(floorMod(d+4, 7)) + 1 } // Sunday is 1
	switch {
	case dow == 0:
		return first + int64(day) - 1
	case dow > 0:
		if day > 0 {
			d := first + int64(day-1)*7
			delta := floorMod(int64(dow-weekday(first)), 7)
			return d + delta
		}
		last := first + int64(length) - 1
		delta := floorMod(int64(weekday(last)-dow), 7)
		return last - delta + int64(day+1)*7
	default:
		target := -dow
		if day > 0 {
			d := first + int64(day) - 1
			return d + floorMod(int64(target-weekday(d)), 7)
		}
		d := first + int64(-day) - 1
		return d - floorMod(int64(weekday(d)-target), 7)
	}
}

func modeToUTC(wallMs int64, mode int, raw, dst int64) int64 {
	switch mode {
	case 0:
		return wallMs - raw - dst
	case 1:
		return wallMs - raw
	}
	return wallMs
}

// ruleTransitions returns, in UTC milliseconds, the rule's transitions into and out of daylight time in a year.
func (z *timeZone) ruleTransitions(year int64) (start, end int64) {
	r := z.finalRule
	raw := z.finalRaw * 1000
	start = modeToUTC(z.finalRule.dayFor(year, r.startMonth, r.startDay, r.startDOW)*86400000+int64(r.startTime), r.startMode, raw, 0)
	end = modeToUTC(z.finalRule.dayFor(year, r.endMonth, r.endDay, r.endDOW)*86400000+int64(r.endTime), r.endMode, raw, int64(r.savings))
	return start, end
}

func (z *timeZone) finalOffsets(ms int64) (raw, dst int64) {
	raw = z.finalRaw * 1000
	y, _, _ := civilFromDays(floorDiv(ms+raw, 86400000))
	start, end := z.ruleTransitions(y)
	in := false
	if start < end {
		in = ms >= start && ms < end
	} else {
		in = ms >= start || ms < end
	}
	if in {
		return raw, int64(z.finalRule.savings)
	}
	return raw, 0
}

func (z *timeZone) typeAt(i int) (raw, dst int64) {
	return z.typeOffsets[2*i] * 1000, z.typeOffsets[2*i+1] * 1000
}

// offsets is getOffset(date, false, raw, dst).
func (z *timeZone) offsets(ms int64) (raw, dst int64) {
	if z.custom {
		return z.fixedOffset, 0
	}
	if z.finalRule != nil && ms >= daysFromCivil(z.finalYear, 1, 1)*86400000 {
		return z.finalOffsets(ms)
	}
	sec := floorDiv(ms, 1000)
	i := sort.Search(len(z.trans), func(i int) bool { return z.trans[i] > sec }) - 1
	if i < 0 {
		return z.typeAt(0)
	}
	return z.typeAt(int(z.typeMap[i]))
}

// wallOffsets is getOffset(wall, true, raw, dst) for a golden zone check, as ICU's local lookup does for the former
// occurrence of a repeated time.
func (z *timeZone) wallOffsets(wall int64) (raw, dst int64) {
	raw1, dst1 := z.offsets(wall - 86400000)
	guess := wall - raw1 - dst1
	raw2, dst2 := z.offsets(guess)
	if guess+raw2+dst2 == wall {
		return raw2, dst2
	}
	raw3, dst3 := z.offsets(wall - raw2 - dst2)
	return raw3, dst3
}

// previousTransition finds the latest change of offsets at or before ms; from is the savings before it.
func (z *timeZone) previousTransition(ms int64) (at, fromSavings int64, ok bool) {
	if z.custom {
		return 0, 0, false
	}
	if z.finalRule != nil && ms >= daysFromCivil(z.finalYear, 1, 1)*86400000 {
		y, _, _ := civilFromDays(floorDiv(ms, 86400000))
		for year := y + 1; year >= z.finalYear; year-- {
			start, end := z.ruleTransitions(year)
			list := [][2]int64{{start, 0}, {end, int64(z.finalRule.savings)}}
			sort.Slice(list, func(a, b int) bool { return list[a][0] > list[b][0] })
			for _, t := range list {
				if t[0] <= ms {
					return t[0], t[1], true
				}
			}
		}
	}
	sec := floorDiv(ms, 1000)
	i := sort.Search(len(z.trans), func(i int) bool { return z.trans[i] > sec }) - 1
	for ; i >= 0; i-- {
		fromRaw, fromDst := z.typeBefore(i)
		toRaw, toDst := z.typeAt(int(z.typeMap[i]))
		if fromRaw != toRaw || fromDst != toDst {
			return z.trans[i] * 1000, fromDst, true
		}
	}
	return 0, 0, false
}

func (z *timeZone) typeBefore(i int) (raw, dst int64) {
	if i <= 0 {
		return z.typeAt(0)
	}
	return z.typeAt(int(z.typeMap[i-1]))
}

// nextTransition finds the first change of offsets after ms; to is the savings after it.
func (z *timeZone) nextTransition(ms int64) (at, toSavings int64, ok bool) {
	if z.custom {
		return 0, 0, false
	}
	finalStart := int64(math.MaxInt64)
	if z.finalRule != nil {
		finalStart = daysFromCivil(z.finalYear, 1, 1) * 86400000
	}
	if ms < finalStart {
		sec := floorDiv(ms, 1000)
		i := sort.Search(len(z.trans), func(i int) bool { return z.trans[i] > sec })
		for ; i < len(z.trans); i++ {
			fromRaw, fromDst := z.typeBefore(i)
			toRaw, toDst := z.typeAt(int(z.typeMap[i]))
			if fromRaw != toRaw || fromDst != toDst {
				return z.trans[i] * 1000, toDst, true
			}
		}
	}
	if z.finalRule == nil {
		return 0, 0, false
	}
	y, _, _ := civilFromDays(floorDiv(max(ms, finalStart), 86400000))
	for year := max(y-1, z.finalYear); year <= y+2; year++ {
		start, end := z.ruleTransitions(year)
		list := [][2]int64{{start, int64(z.finalRule.savings)}, {end, 0}}
		sort.Slice(list, func(a, b int) bool { return list[a][0] < list[b][0] })
		for _, t := range list {
			if t[0] > ms && t[0] >= finalStart {
				return t[0], t[1], true
			}
		}
	}
	return 0, 0, false
}
