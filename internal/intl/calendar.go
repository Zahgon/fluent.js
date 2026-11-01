package intl

import (
	"math"
	"sort"
	"sync"

	"github.com/projectfluent/fluent-go/internal/icudata"
)

const (
	epochJD        = 2440588 // ICU Julian day of 1970-01-01
	gregorianShift = 2299161 // ICU Julian day of 1582-10-15
)

// calFields are the calendar fields ICU formats for one local date. Month is ICU's zero-based month field; for the
// Hebrew calendar it indexes Tishri .. Elul with Adar I at 5.
type calFields struct {
	era   int
	year  int64
	month int
	day   int64
	leap  bool
}

func floorDiv(a, b int64) int64 {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}

func floorMod(a, b int64) int64 { return a - floorDiv(a, b)*b }

// civilFromDays converts days since 1970-01-01 to the proleptic Gregorian calendar.
func civilFromDays(z int64) (y, m, d int64) {
	z += 719468
	era := floorDiv(z, 146097)
	doe := z - era*146097
	yoe := (doe - doe/1460 + doe/36524 - doe/146096) / 365
	doy := doe - (365*yoe + yoe/4 - yoe/100)
	mp := (5*doy + 2) / 153
	d = doy - (153*mp+2)/5 + 1
	if mp < 10 {
		m = mp + 3
	} else {
		m = mp - 9
	}
	y = yoe + era*400
	if m <= 2 {
		y++
	}
	return y, m, d
}

// daysFromCivil is the inverse of civilFromDays.
func daysFromCivil(y, m, d int64) int64 {
	if m <= 2 {
		y--
	}
	era := floorDiv(y, 400)
	yoe := y - era*400
	mp := m + 9
	if m > 2 {
		mp = m - 3
	}
	doy := (153*mp+2)/5 + d - 1
	doe := yoe*365 + yoe/4 - yoe/100 + doy
	return era*146097 + doe - 719468
}

var cumDays = [2][12]int64{
	{0, 31, 59, 90, 120, 151, 181, 212, 243, 273, 304, 334},
	{0, 31, 60, 91, 121, 152, 182, 213, 244, 274, 305, 335},
}

// gregorianJulianFields is ICU's GregorianCalendar with its default 1582-10-15 switch from the Julian calendar; only
// a plain Gregorian calendar gets the proleptic rule from V8.
func gregorianJulianFields(jd int64) (eyear, month, day int64) {
	if jd >= gregorianShift {
		return civilFromDays(jd - epochJD)
	}
	jed := jd - (1721426 - 2)
	eyear = int64(math.Floor((4*float64(jed) + 1464) / 1461))
	jan1 := 365*(eyear-1) + floorDiv(eyear-1, 4)
	doy := jed - jan1
	leap := eyear&3 == 0
	corr := int64(0)
	march1 := int64(59)
	if leap {
		march1 = 60
	}
	if doy >= march1 {
		if leap {
			corr = 1
		} else {
			corr = 2
		}
	}
	m := (12*(doy+corr) + 6) / 367
	li := 0
	if leap {
		li = 1
	}
	return eyear, m + 1, doy - cumDays[li][m] + 1
}

type japaneseEra struct {
	index      int
	start      int64 // days since 1970-01-01
	startYear  int64
	startMonth int64
	startDay   int64
}

var (
	calendarOnce sync.Once
	japEras      []japaneseEra
	persianFirst int64
	persianStart []int64
	uqStarts     [][3]int64
)

func loadCalendarData() {
	calendarOnce.Do(func() {
		data := icudata.Table("supp/calendar")
		eras, _ := data["japaneseEras"].([]any)
		for i, e := range eras {
			v := e.([]any)
			y, m, d := int64(v[0].(float64)), int64(v[1].(float64)), int64(v[2].(float64))
			japEras = append(japEras, japaneseEra{index: i, startYear: y, startMonth: m, startDay: d})
		}
		persian, _ := data["persian"].(map[string]any)
		persianFirst = int64(persian["first"].(float64))
		for _, v := range persian["starts"].([]any) {
			persianStart = append(persianStart, int64(v.(float64)))
		}
		for _, v := range data["umalqura"].([]any) {
			m := v.([]any)
			uqStarts = append(uqStarts, [3]int64{int64(m[0].(float64)), int64(m[1].(float64)), int64(m[2].(float64))})
		}
	})
}

// calendarFields computes the fields of a local day number (days since 1970-01-01) in an Intl calendar.
func calendarFieldsFor(calendar string, day int64) calFields {
	jd := day + epochJD
	switch calendar {
	case "buddhist":
		y, m, d := gregorianJulianFields(jd)
		return calFields{era: 0, year: y + 543, month: int(m - 1), day: d}
	case "roc":
		y, m, d := gregorianJulianFields(jd)
		y -= 1911
		if y > 0 {
			return calFields{era: 1, year: y, month: int(m - 1), day: d}
		}
		return calFields{era: 0, year: 1 - y, month: int(m - 1), day: d}
	case "japanese":
		loadCalendarData()
		y, m, d := gregorianJulianFields(jd)
		idx := 0
		for i := len(japEras) - 1; i >= 0; i-- {
			e := japEras[i]
			if y > e.startYear || y == e.startYear && (m > e.startMonth || m == e.startMonth && d >= e.startDay) {
				idx = i
				break
			}
		}
		return calFields{era: idx, year: y - japEras[idx].startYear + 1, month: int(m - 1), day: d}
	case "persian":
		return persianFields(jd)
	case "hebrew":
		return hebrewFields(jd)
	case "indian":
		return indianFields(day)
	case "coptic":
		y, m, d := ceFields(jd, 1824665)
		if y <= 0 {
			return calFields{era: 0, year: 1 - y, month: int(m), day: d}
		}
		return calFields{era: 1, year: y, month: int(m), day: d}
	case "ethiopic":
		y, m, d := ceFields(jd, 1723856)
		if y > 0 {
			return calFields{era: 1, year: y, month: int(m), day: d}
		}
		return calFields{era: 0, year: y + 5500, month: int(m), day: d}
	case "ethioaa":
		y, m, d := ceFields(jd, 1723856)
		return calFields{era: 0, year: y + 5500, month: int(m), day: d}
	case "islamic-civil":
		return islamicCivil(jd, 1948440)
	case "islamic-tbla":
		return islamicCivil(jd, 1948439)
	case "islamic-umalqura":
		loadCalendarData()
		if len(uqStarts) > 0 && day >= uqStarts[0][0] && day < uqStarts[len(uqStarts)-1][0] {
			i := sort.Search(len(uqStarts), func(i int) bool { return uqStarts[i][0] > day }) - 1
			u := uqStarts[i]
			return calFields{era: 0, year: u[1], month: int(u[2]), day: day - u[0] + 1}
		}
		return islamicCivil(jd, 1948440)
	}
	y, m, d := civilFromDays(day)
	if y <= 0 {
		return calFields{era: 0, year: 1 - y, month: int(m - 1), day: d}
	}
	return calFields{era: 1, year: y, month: int(m - 1), day: d}
}

var persianCum = [12]int64{0, 31, 62, 93, 124, 155, 186, 216, 246, 276, 306, 336}

func persianFields(jd int64) calFields {
	loadCalendarData()
	since := jd - 1948320
	var year, doy int64
	day := jd - epochJD
	if len(persianStart) > 1 && day >= persianStart[0] && day < persianStart[len(persianStart)-1] {
		i := sort.Search(len(persianStart), func(i int) bool { return persianStart[i] > day }) - 1
		year, doy = persianFirst+int64(i), day-persianStart[i]
	} else {
		year = 1 + floorDiv(33*since+3, 12053)
		doy = since - (365*(year-1) + floorDiv(8*year+21, 33))
	}
	var month int64
	if doy < 216 {
		month = doy / 31
	} else {
		month = (doy - 6) / 30
	}
	return calFields{era: 0, year: year, month: int(month), day: doy - persianCum[month] + 1}
}

func ceFields(jd, epoch int64) (year, month, day int64) {
	c4 := floorDiv(jd-epoch, 1461)
	r4 := floorMod(jd-epoch, 1461)
	year = 4*c4 + (r4/365 - r4/1460)
	doy := r4 % 365
	if r4 == 1460 {
		doy = 365
	}
	return year, doy / 30, doy%30 + 1
}

func islamicYearStart(year int64) int64 { return (year-1)*354 + floorDiv(3+11*year, 30) }

func islamicCivil(jd, epoch int64) calFields {
	days := jd - epoch
	year := floorDiv(30*days+10646, 10631)
	month := int64(math.Ceil(float64(days-29-islamicYearStart(year)) / 29.5))
	if month > 11 {
		month = 11
	}
	start := int64(math.Ceil(29.5*float64(month))) + islamicYearStart(year)
	return calFields{era: 0, year: year, month: int(month), day: days - start + 1}
}

func indianFields(day int64) calFields {
	gy, _, _ := civilFromDays(day)
	year := gy - 78
	yday := day - daysFromCivil(gy, 1, 1)
	leap := func(y int64) bool { return (y%4 == 0 && y%100 != 0) || y%400 == 0 }
	var leapMonth int64
	if yday < 80 {
		year--
		leapMonth = 30
		if leap(gy - 1) {
			leapMonth = 31
		}
		yday += leapMonth + 31*5 + 30*3 + 10
	} else {
		leapMonth = 30
		if leap(gy) {
			leapMonth = 31
		}
		yday -= 80
	}
	var month, dom int64
	if yday < leapMonth {
		month, dom = 0, yday+1
	} else {
		mday := yday - leapMonth
		if mday < 31*5 {
			month, dom = mday/31+1, mday%31+1
		} else {
			mday -= 31 * 5
			month, dom = mday/30+6, mday%30+1
		}
	}
	return calFields{era: 0, year: year, month: int(month), day: dom}
}

const (
	hHourParts  = 1080
	hDayParts   = 24 * hHourParts
	hMonthFract = 12*hHourParts + 793
	hMonthParts = 29*hDayParts + hMonthFract
	hBaharad    = 11*hHourParts + 204
)

var hebrewMonthStart = [14][3]int64{
	{0, 0, 0}, {30, 30, 30}, {59, 59, 60}, {88, 89, 90}, {117, 118, 119}, {147, 148, 149},
	{147, 148, 149}, {176, 177, 178}, {206, 207, 208}, {235, 236, 237}, {265, 266, 267},
	{294, 295, 296}, {324, 325, 326}, {353, 354, 355},
}

var hebrewLeapMonthStart = [14][3]int64{
	{0, 0, 0}, {30, 30, 30}, {59, 59, 60}, {88, 89, 90}, {117, 118, 119}, {147, 148, 149},
	{177, 178, 179}, {206, 207, 208}, {236, 237, 238}, {265, 266, 267}, {295, 296, 297},
	{324, 325, 326}, {354, 355, 356}, {383, 384, 385},
}

func hebrewLeap(year int64) bool {
	x := (year*12 + 17) % 19
	if x < 0 {
		return x >= -7
	}
	return x >= 12
}

func hebrewStartOfYear(year int64) int64 {
	months := floorDiv(235*year-234, 19)
	frac := months*hMonthFract + hBaharad
	day := months*29 + frac/hDayParts
	frac = frac % hDayParts
	switch wd := day % 7; {
	case wd == 2 || wd == 4 || wd == 6:
		day++
	case wd == 1 && frac > 15*hHourParts+204 && !hebrewLeap(year):
		day += 2
	case wd == 0 && frac > 21*hHourParts+589 && hebrewLeap(year-1):
		day++
	}
	return day
}

func hebrewYearType(year int64) int {
	l := hebrewStartOfYear(year+1) - hebrewStartOfYear(year)
	if l > 380 {
		l -= 30
	}
	switch l {
	case 353:
		return 0
	case 355:
		return 2
	}
	return 1
}

func hebrewFields(jd int64) calFields {
	d := jd - 347997
	m := math.Floor(float64(d) * hDayParts / hMonthParts)
	year := int64(math.Floor((float64(19*m)+234)/235) + 1)
	ys := hebrewStartOfYear(year)
	doy := d - ys
	for doy < 1 {
		year--
		ys = hebrewStartOfYear(year)
		doy = d - ys
	}
	typ := hebrewYearType(year)
	leap := hebrewLeap(year)
	table := &hebrewMonthStart
	if leap {
		table = &hebrewLeapMonthStart
	}
	month := 0
	for month < 14 && doy > table[month][typ] {
		month++
	}
	month--
	if month < 0 {
		month = 0
	}
	return calFields{era: 0, year: year, month: month, day: doy - table[month][typ], leap: leap}
}
