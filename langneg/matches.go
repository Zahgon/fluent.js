// Ported from fluent.js (Apache-2.0), fluent-langneg/src/matches.ts.

package langneg

import (
	"strings"

	"github.com/projectfluent/fluent-go/internal/intl"
)

// availableLocale is an entry of the original's availableLocalesMap.
type availableLocale struct {
	key      string
	variants *string
	subtags  subtags
	matched  bool
}

// matcher is the state of one FilterMatches call.
type matcher struct {
	strategy  string
	available []*availableLocale
	supported []string
	// finished is set when the lookup strategy has its one match.
	finished bool
}

// FilterMatches returns the available locales that match the requested ones, following
// BCP47 extended filtering with three additions: available locales are ranges, likely
// subtags are added to requested locales, and a same-language match in another region or
// variant is accepted. The strategy "filtering" keeps every match, "lookup" stops at the
// first one, and anything else keeps the best match of each requested locale.
func FilterMatches(requested, available []string, strategy string) []string {
	m := &matcher{strategy: strategy, supported: []string{}}

	byKey := map[string]*availableLocale{}
	for _, key := range available {
		wrapper, ok := tryNewLocaleWrapper(key)
		if !ok {
			continue
		}
		// A repeated key keeps its position and takes the last wrapper, as in a Map.
		entry := byKey[key]
		if entry == nil {
			entry = &availableLocale{key: key}
			byKey[key] = entry
			m.available = append(m.available, entry)
		}
		entry.variants, entry.subtags = wrapper.variants, wrapper.subtags()
	}

	for _, tag := range requested {
		if wrapper, ok := tryNewLocaleWrapper(tag); ok {
			m.match(tag, wrapper)
			if m.finished {
				break
			}
		}
	}
	return m.supported
}

// match runs the steps of the algorithm for one requested locale, up to its first match
// unless the strategy is filtering.
func (m *matcher) match(tag string, requested *localeWrapper) {
	// 1) An exact match. Example: en-US === en-US.
	lowerTag := strings.ToLower(tag)
	if m.scan(func(a *availableLocale) bool { return lowerTag == strings.ToLower(a.key) }) {
		return
	}

	reqVariants := requested.variants
	sameVariants := func(a *availableLocale) bool {
		return a.variants == nil || variantsEqual(a.variants, reqVariants)
	}

	// 2) Match against the available range. Example: ['en-US'] * ['en'] = ['en'].
	current := requested.subtags()
	if m.scan(func(a *availableLocale) bool {
		return a.subtags.matches(current, false) && sameVariants(a)
	}) {
		return
	}

	// 3) Match a maximal version of the requested locale.
	// Example: ['en'] * ['en-GB', 'en-US'] = ['en-US'].
	current = bareSubtags(requested.locale.Maximize())
	if m.scan(func(a *availableLocale) bool {
		return a.subtags.matches(current, false) && sameVariants(a)
	}) {
		return
	}

	// 4) A different variant of the same locale.
	// Example: ['en-US-mac'] * ['en-US-win'] = ['en-US-win'].
	if m.scan(func(a *availableLocale) bool { return a.subtags.matches(current, true) }) {
		return
	}

	// 5) The likely subtags without the region.
	// Example: ['zh-Hant-HK'] * ['zh-TW', 'zh-CN'] = ['zh-TW'].
	current = bareSubtags(intl.NewLocaleWithScript(current.language, current.script).Maximize())
	if m.scan(func(a *availableLocale) bool { return a.subtags.matches(current, false) }) {
		return
	}

	// 6) A different region of the same locale. Example: ['en-US'] * ['en-AU'] = ['en-AU'].
	m.scan(func(a *availableLocale) bool {
		return a.subtags.languageMatches(current, true) && a.subtags.scriptMatches(current, true)
	})
}

// scan takes the unmatched available locales the predicate accepts, in order. It reports
// whether the requested locale is done with: filtering goes on to the next step, and any
// other strategy stops at the first match.
func (m *matcher) scan(accepts func(*availableLocale) bool) bool {
	for _, entry := range m.available {
		if entry.matched || !accepts(entry) {
			continue
		}
		m.supported = append(m.supported, entry.key)
		entry.matched = true
		if m.strategy != "filtering" {
			m.finished = m.strategy == "lookup"
			return true
		}
	}
	return false
}

func variantsEqual(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// matches compares all three subtags of an available locale with a requested one.
func (a subtags) matches(b subtags, isRange bool) bool {
	return a.languageMatches(b, isRange) && a.scriptMatches(b, isRange) && a.regionMatches(b, isRange)
}

func (a subtags) languageMatches(b subtags, isRange bool) bool {
	return a.language == b.language || a.language == "und" || (isRange && b.language == "und")
}

func (a subtags) scriptMatches(b subtags, isRange bool) bool {
	return a.script == b.script || a.script == "" || (isRange && b.script == "")
}

func (a subtags) regionMatches(b subtags, isRange bool) bool {
	return a.region == b.region || a.region == "" || (isRange && b.region == "")
}
