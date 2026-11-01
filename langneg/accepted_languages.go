// Ported from fluent.js (Apache-2.0), fluent-langneg/src/accepted_languages.ts.

package langneg

import (
	"cmp"
	"math"
	"slices"
	"strings"

	"github.com/projectfluent/fluent-go/internal/jsnum"
)

// acceptedLanguage is one entry of the header.
type acceptedLanguage struct {
	lang  string
	q     float64
	index int
}

// AcceptedLanguages returns the languages of an Accept-Language header, by descending q
// and in header order for equal weights. An entry with an unparseable q has a weight of 0.
func AcceptedLanguages(header string) []string {
	langs := parseAcceptLanguage([]rune(header))

	slices.SortStableFunc(langs, func(a, b acceptedLanguage) int {
		if a.q == b.q {
			return cmp.Compare(a.index, b.index)
		}
		return cmp.Compare(b.q, a.q)
	})

	result := make([]string, len(langs))
	for i, lang := range langs {
		result[i] = lang.lang
	}
	return result
}

// parseAcceptLanguage is matchAll of /(?:^|,)([^,;]+)(?:;\s*[qQ]\s*=([^,;]+))?/g.
func parseAcceptLanguage(header []rune) []acceptedLanguage {
	langs := []acceptedLanguage{}
	for start := 0; start < len(header); {
		// Only the very beginning of the header, or a comma, can start an entry.
		langStart := start
		if header[start] == ',' {
			langStart++
		} else if start != 0 {
			start++
			continue
		}
		langEnd := delimitedEnd(header, langStart)
		if langEnd == langStart {
			start++
			continue
		}

		lang := strings.TrimFunc(string(header[langStart:langEnd]), jsnum.IsSpace)
		q, end := parseWeight(header, langEnd)
		langs = append(langs, acceptedLanguage{lang: lang, q: q, index: start})
		start = end
	}
	return langs
}

func isEntryDelimiter(ch rune) bool { return ch == ',' || ch == ';' }

// delimitedEnd matches [^,;]* at i and returns its end.
func delimitedEnd(header []rune, i int) int {
	for i < len(header) && !isEntryDelimiter(header[i]) {
		i++
	}
	return i
}

// parseWeight matches (?:;\s*[qQ]\s*=([^,;]+))? at i. It returns the weight and the end of
// the match. The weight is 1 if the group does not match, and `parseFloat(q) || 0` if it does.
func parseWeight(header []rune, i int) (float64, int) {
	if i == len(header) || header[i] != ';' {
		return 1, i
	}
	j := spaceEnd(header, i+1)
	if j == len(header) || (header[j] != 'q' && header[j] != 'Q') {
		return 1, i
	}
	j = spaceEnd(header, j+1)
	if j == len(header) || header[j] != '=' {
		return 1, i
	}
	valueEnd := delimitedEnd(header, j+1)
	if valueEnd == j+1 {
		return 1, i
	}
	q := jsnum.ParseFloat(string(header[j+1 : valueEnd]))
	if math.IsNaN(q) || q == 0 {
		q = 0
	}
	return q, valueEnd
}

// spaceEnd matches \s* at i and returns its end.
func spaceEnd(header []rune, i int) int {
	for i < len(header) && jsnum.IsSpace(header[i]) {
		i++
	}
	return i
}
