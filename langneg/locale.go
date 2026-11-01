// Ported from fluent.js (Apache-2.0), fluent-langneg/src/locale.ts.

package langneg

import (
	"strings"

	"github.com/projectfluent/fluent-go"
	"github.com/projectfluent/fluent-go/internal/intl"
)

// localeWrapper is the original's LocaleWrapper: an Intl.Locale built from a rewritten
// tag, with the variants polyfill. The target runtime has no
// Intl.Locale.prototype.variants, so the polyfill branch is the one that always runs.
type localeWrapper struct {
	locale *intl.Locale
	// language falls back to "und", unlike the bare locale's.
	language string
	// variants is computed from the rewritten tag, not the canonicalized one. nil is
	// undefined; the empty string is possible when canonicalization shortens the tag.
	variants *string
}

// newLocaleWrapper wraps a tag; an invalid tag raises the RangeError of Intl.Locale.
func newLocaleWrapper(locale string) *localeWrapper {
	tag := strings.ReplaceAll(locale, "_", "-")
	if strings.HasPrefix(tag, "*") {
		tag = "und" + tag[1:]
	}
	tag = strings.ReplaceAll(tag, "-*", "")

	wrapper := &localeWrapper{locale: intl.NewLocale(tag), language: "und"}
	if language := wrapper.locale.Language(); language != "" {
		wrapper.language = language
	}

	lsrTagLength := len(wrapper.language)
	if script := wrapper.locale.Script(); script != "" {
		lsrTagLength += len(script) + 1
	}
	if region := wrapper.locale.Region(); region != "" {
		lsrTagLength += len(region) + 1
	}
	if len(tag) > lsrTagLength {
		variants := substring(tag, lsrTagLength+1, extensionStart(tag))
		wrapper.variants = &variants
	}
	return wrapper
}

// tryNewLocaleWrapper is the original's try/catch around a new LocaleWrapper.
func tryNewLocaleWrapper(locale string) (wrapper *localeWrapper, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			if _, isException := r.(*fluent.Error); !isException {
				panic(r)
			}
			wrapper, ok = nil, false
		}
	}()
	return newLocaleWrapper(locale), true
}

// extensionStart is the first match of /-[a-zA-Z]-/ in tag, or the tag's length.
func extensionStart(tag string) int {
	for i := 0; i+2 < len(tag); i++ {
		letter := tag[i+1] | 0x20
		if tag[i] == '-' && 'a' <= letter && letter <= 'z' && tag[i+2] == '-' {
			return i
		}
	}
	return len(tag)
}

// substring is String.prototype.substring on a tag that Intl.Locale accepted, so it is
// ASCII: out-of-range indices clamp and swapped ones change places, so the polyfill's
// variants of "en-u-ca-gregory" are "-".
func substring(s string, start, end int) string {
	start, end = min(start, len(s)), min(end, len(s))
	if start > end {
		start, end = end, start
	}
	return s[start:end]
}

// subtags is what the language, script and region matching compares. "" is undefined.
type subtags struct{ language, script, region string }

// subtags returns the wrapper's subtags, with the language fallback.
func (w *localeWrapper) subtags() subtags {
	return subtags{language: w.language, script: w.locale.Script(), region: w.locale.Region()}
}

// bareSubtags returns the subtags of a locale that is not a wrapper.
func bareSubtags(locale *intl.Locale) subtags {
	return subtags{language: locale.Language(), script: locale.Script(), region: locale.Region()}
}
