package intl

import (
	"strings"

	"github.com/projectfluent/fluent-go/internal/icudata"
)

const dstCheckRange = int64(184 * 24 * 3600 * 1000)

func zoneKey(id string) string { return strings.ReplaceAll(id, "/", ":") }

var canonicalZoneSet map[string]bool

// canonicalIDOf is ZoneMeta::getCanonicalCLDRID: a CLDR alias, a CLDR canonical id, or the same test on the link's target.
func canonicalIDOf(id string) string {
	data := icudata.Table("supp/zoneAliases")
	if canonicalZoneSet == nil {
		canonicalZoneSet = map[string]bool{}
		list, _ := data["canonical"].([]any)
		for _, v := range list {
			canonicalZoneSet[v.(string)] = true
		}
	}
	aliases, _ := data["alias"].(map[string]any)
	for {
		if a, ok := aliases[zoneKey(id)].(string); ok {
			return a
		}
		if canonicalZoneSet[zoneKey(id)] {
			return id
		}
		zoneIndex()
		idx, ok := zoneIndexCache[id]
		if !ok {
			return id
		}
		names, zones, _, _ := zoneTables()
		n, isLink := zones[idx].(float64)
		if !isLink {
			return id
		}
		id = names[int(n)].(string)
	}
}

func canonicalZoneID(z *timeZone) string {
	if z.custom {
		return ""
	}
	return canonicalIDOf(z.id)
}

func (f *DateTimeFormat) zoneString(name string) string {
	return lookupString("zone", f.locale, "zoneStrings", name)
}

func (f *DateTimeFormat) zoneName(id, kind string) string {
	s := lookupString("zone", f.locale, "zoneStrings", zoneKey(id), kind)
	if s == "∅∅∅" {
		return ""
	}
	return s
}

func (f *DateTimeFormat) metaName(mz, kind string) string {
	s := lookupString("zone", f.locale, "zoneStrings", "meta:"+mz, kind)
	if s == "∅∅∅" {
		return ""
	}
	return s
}

// metaZoneID finds the metazone a canonical zone belongs to at an instant.
func metaZoneID(id string, t int64) string {
	info, _ := icudata.Table("supp/metaZones")["info"].(map[string]any)
	list, _ := info[zoneKey(id)].([]any)
	for _, item := range list {
		v := toStrings(item)
		if len(v) == 0 {
			continue
		}
		from, to := parseMetaDate("1970-01-01 00:00"), parseMetaDate("9999-12-31 23:59")
		if len(v) >= 3 {
			from, to = parseMetaDate(v[1]), parseMetaDate(v[2])
		}
		if t >= from && t < to {
			return v[0]
		}
	}
	return ""
}

func parseMetaDate(s string) int64 {
	var y, mo, d, h, mi int64
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == '-' || r == ' ' || r == ':' })
	if len(parts) < 5 {
		return 0
	}
	y, mo, d, h, mi = atoi64(parts[0]), atoi64(parts[1]), atoi64(parts[2]), atoi64(parts[3]), atoi64(parts[4])
	return daysFromCivil(y, mo, d)*86400000 + h*3600000 + mi*60000
}

func atoi64(s string) int64 {
	var n int64
	for i := 0; i < len(s); i++ {
		n = n*10 + int64(s[i]-'0')
	}
	return n
}

func (f *DateTimeFormat) displayName(id, kind string, t int64) string {
	if s := f.zoneName(id, kind); s != "" {
		return s
	}
	if mz := metaZoneID(id, t); mz != "" {
		return f.metaName(mz, kind)
	}
	return ""
}

func referenceZone(mz, region string) string {
	m, _ := icudata.Table("supp/metaZones")["map"].(map[string]any)
	t, _ := m[mz].(map[string]any)
	if s, ok := t[region].(string); ok {
		return s
	}
	s, _ := t["001"].(string)
	return s
}

func (f *DateTimeFormat) targetRegion() string {
	parts := strings.Split(f.locale, "_")
	id := localeID{lang: parts[0]}
	for _, p := range parts[1:] {
		switch {
		case len(p) == 4:
			id.script = p
		case len(p) == 2 || len(p) == 3:
			id.region = p
		}
	}
	if id.region != "" {
		return id.region
	}
	return maximizeLSR(id.lang, id.script, "", "")[2]
}

func zoneRegion(id string) string {
	zoneIndex()
	idx, ok := zoneIndexCache[id]
	if !ok {
		return ""
	}
	_, _, _, regions := zoneTables()
	r, _ := regions[idx].(string)
	if r == "001" {
		return ""
	}
	return r
}

var regionCanonicalCount map[string]int

func canonicalZonesIn(region string) int {
	if regionCanonicalCount == nil {
		regionCanonicalCount = map[string]int{}
		names, _, _, regions := zoneTables()
		for i, n := range names {
			id := n.(string)
			r, _ := regions[i].(string)
			if r == "" || r == "001" || canonicalIDOf(id) != id {
				continue
			}
			regionCanonicalCount[r]++
		}
	}
	return regionCanonicalCount[region]
}

func (f *DateTimeFormat) isPrimaryZone(id, region string) bool {
	if canonicalZonesIn(region) == 1 {
		return true
	}
	primary, _ := icudata.Table("supp/metaZones")["primary"].(map[string]any)
	p, ok := primary[region].(string)
	return ok && p == id
}

func (f *DateTimeFormat) regionDisplayName(region string) string {
	if s := lookupString("region", f.locale, "Countries", region); s != "" {
		return s
	}
	return region
}

func (f *DateTimeFormat) exemplarCity(id string) string {
	if s := f.zoneName(id, "ec"); s != "" {
		return s
	}
	i := strings.LastIndexByte(id, '/')
	if i < 0 || i+1 >= len(id) || strings.HasPrefix(id, "Etc/") || strings.HasPrefix(id, "SystemV/") {
		return ""
	}
	return strings.ReplaceAll(id[i+1:], "_", " ")
}

func (f *DateTimeFormat) genericLocation(id string) string {
	region := zoneRegion(id)
	if region == "" {
		return ""
	}
	regionFormat := f.zoneString("regionFormat")
	if f.isPrimaryZone(id, region) {
		return strings.ReplaceAll(regionFormat, "{0}", f.regionDisplayName(region))
	}
	return strings.ReplaceAll(regionFormat, "{0}", f.exemplarCity(id))
}

func (f *DateTimeFormat) partialLocation(id, mz, mzName string) string {
	location := ""
	if region := zoneRegion(id); region != "" {
		if id == referenceZone(mz, region) {
			location = f.regionDisplayName(region)
		} else {
			location = f.exemplarCity(id)
		}
	} else {
		location = f.exemplarCity(id)
		if location == "" {
			location = id
		}
	}
	fallback := f.zoneString("fallbackFormat")
	return strings.ReplaceAll(strings.ReplaceAll(fallback, "{0}", location), "{1}", mzName)
}

func (f *DateTimeFormat) genericName(z *timeZone, long bool, t int64, c *dateContext) string {
	id := canonicalZoneID(z)
	if id == "" {
		return ""
	}
	genericKind, standardKind := "sg", "ss"
	if long {
		genericKind, standardKind = "lg", "ls"
	}
	if s := f.zoneName(id, genericKind); s != "" {
		return s
	}
	mz := metaZoneID(id, t)
	if mz == "" {
		return ""
	}
	useStandard := false
	if c.dst == 0 {
		useStandard = true
		if at, from, ok := z.previousTransition(t); ok && t-at < dstCheckRange && from != 0 {
			useStandard = false
		} else if at, to, ok := z.nextTransition(t); ok && at-t < dstCheckRange && to != 0 {
			useStandard = false
		}
	}
	if useStandard {
		std := f.displayName(id, standardKind, t)
		if std != "" && !strings.EqualFold(std, f.metaName(mz, genericKind)) {
			return std
		}
	}
	name := f.metaName(mz, genericKind)
	if name == "" {
		return ""
	}
	golden := referenceZone(mz, f.targetRegion())
	if golden != "" && golden != id {
		gz := loadZone(golden)
		if gz != nil {
			raw1, dst1 := gz.wallOffsets(t + c.raw + c.dst)
			if c.raw != raw1 || c.dst != dst1 {
				return f.partialLocation(id, mz, name)
			}
		}
	}
	return name
}

func (f *DateTimeFormat) zoneText(letter byte, count int, c *dateContext) string {
	z := f.zone
	switch letter {
	case 'z':
		long := count == 4
		kind := "ss"
		if long {
			kind = "ls"
		}
		if c.dst != 0 {
			kind = "sd"
			if long {
				kind = "ld"
			}
		}
		if id := canonicalZoneID(z); id != "" {
			if s := f.displayName(id, kind, c.t); s != "" {
				return s
			}
		}
		return f.localizedGMT(c.raw+c.dst, !long)
	case 'v':
		long := count == 4
		s := f.genericName(z, long, c.t, c)
		if s == "" {
			if id := canonicalZoneID(z); id != "" {
				s = f.genericLocation(id)
			}
		}
		if s != "" {
			return s
		}
		return f.localizedGMT(c.raw+c.dst, !long)
	case 'O':
		return f.localizedGMT(c.raw+c.dst, count == 1)
	}
	return ""
}

func (f *DateTimeFormat) localizedGMT(offset int64, short bool) string {
	hourFormat := f.zoneString("hourFormat")
	pos, neg, _ := strings.Cut(hourFormat, ";")
	pattern := pos
	if offset < 0 {
		pattern = neg
		offset = -offset
	}
	h := offset / 3600000
	m := offset / 60000 % 60
	s := offset / 1000 % 60
	switch {
	case s != 0:
		pattern = expandHourPattern(pattern)
	case m == 0 && short:
		pattern = truncateHourPattern(pattern)
	}
	var b strings.Builder
	items := patternItems(pattern)
	for _, item := range items {
		switch item[0] {
		case 'H':
			if short {
				b.WriteString(f.numberIn(f.digits, h, 1))
			} else {
				b.WriteString(f.numberIn(f.digits, h, 2))
			}
		case 'm':
			b.WriteString(f.numberIn(f.digits, m, 2))
		case 's':
			b.WriteString(f.numberIn(f.digits, s, 2))
		default:
			b.WriteString(item)
		}
	}
	gmt := f.zoneString("gmtFormat")
	return strings.ReplaceAll(gmt, "{0}", b.String())
}

func expandHourPattern(p string) string {
	i := strings.Index(p, "mm")
	if i < 0 {
		return p
	}
	sep := ""
	if h := strings.LastIndexByte(p[:i], 'H'); h >= 0 {
		sep = p[h+1 : i]
	}
	return p[:i+2] + sep + "ss" + p[i+2:]
}

func truncateHourPattern(p string) string {
	i := strings.LastIndexByte(p, 'H')
	if i < 0 {
		return p
	}
	return p[:i+1]
}
