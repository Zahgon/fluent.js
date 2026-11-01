package intl

var (
	hebrewOnes     = []string{"", "א", "ב", "ג", "ד", "ה", "ו", "ז", "ח", "ט"}
	hebrewTens     = []string{"", "י", "כ", "ל", "מ", "נ", "ס", "ע", "פ", "צ"}
	hebrewHundreds = []string{"", "ק", "ר", "ש", "ת", "תק", "תר", "תש", "תת", "תתק"}
	hebrewSpecial  = map[int64]string{15: "טו", 16: "טז"}
	// Letter sequences that would spell a word of ill omen are reordered (304 only, not 704).
	hebrewReorder = map[int64]string{298: "רחצ", 698: "תרחצ", 344: "שדמ", 744: "תשדמ", 304: "דש"}
)

const (
	geresh    = "\u05F3"
	gershayim = "\u05F4"
)

func hebrewLetters(n int64) string {
	if s, ok := hebrewReorder[n]; ok {
		return s
	}
	h := hebrewHundreds[n/100]
	rest := n % 100
	if s, ok := hebrewSpecial[rest]; ok {
		return h + s
	}
	if h != "" && rest == 80 {
		return h + "ף"
	}
	return h + hebrewTens[rest/10] + hebrewOnes[rest%10]
}

func hebrewPunctuate(letters string) string {
	r := []rune(letters)
	if len(r) == 1 {
		return letters + geresh
	}
	return string(r[:len(r)-1]) + gershayim + string(r[len(r)-1])
}

func hebrewNumber(n int64) string {
	if n == 0 {
		return "אפס"
	}
	if n < 1000 {
		return hebrewPunctuate(hebrewLetters(n))
	}
	k, rem := n/1000, n%1000
	if rem == 0 {
		switch k {
		case 1:
			return "אלף"
		case 2:
			return "אלפיים"
		}
		return hebrewNumber(k) + " אלפים"
	}
	prefix := hebrewNumber(k) + geresh
	if k <= 10 || k == 100 {
		prefix = hebrewLetters(k) + geresh
	}
	return prefix + hebrewNumber(rem)
}

// hebrewNumeral renders a Hebrew calendar year the way ICU does for he with the hebrew calendar:
// years of the current millennium drop the 5000.
func hebrewNumeral(y int64) string {
	if y < 0 {
		return "\u2212" + hebrewNumber(-y)
	}
	if y > 5000 && y < 6000 {
		y -= 5000
	}
	return hebrewNumber(y)
}
