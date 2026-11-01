package intl

import (
	"slices"
	"sort"
	"strings"

	"github.com/projectfluent/fluent-go/internal/icudata"
)

// localeID is a parsed Unicode locale identifier, canonical in case.
type localeID struct {
	lang, script, region string
	variants             []string
	ext                  []string // the extension sequences other than -u-, in order, each starting with its singleton
	uAttrs               []string
	uKeys                []uKeyword
	private              string // "x-..." when present
}

type uKeyword struct{ key, value string }

func isVariant(s string) bool {
	if len(s) >= 5 && len(s) <= 8 {
		return isASCIIAlnum(s)
	}
	return len(s) == 4 && s[0] >= '0' && s[0] <= '9' && isASCIIAlnum(s)
}

// parseLocaleTag parses a BCP 47 tag under the structural rules of ECMA-402 (a Unicode locale identifier).
func parseLocaleTag(tag string) (*localeID, bool) {
	if tag == "" {
		return nil, false
	}
	parts := strings.Split(tag, "-")
	for _, p := range parts {
		if p == "" || len(p) > 8 || !isASCIIAlnum(p) {
			return nil, false
		}
	}
	id := &localeID{}
	i := 0
	first := parts[0]
	if !isASCIIAlpha(first) || len(first) == 4 || len(first) < 2 {
		return nil, false
	}
	id.lang = strings.ToLower(first)
	i++
	if i < len(parts) && len(parts[i]) == 4 && isASCIIAlpha(parts[i]) {
		id.script = toTitle(parts[i])
		i++
	}
	if i < len(parts) && (len(parts[i]) == 2 && isASCIIAlpha(parts[i]) || len(parts[i]) == 3 && isASCIIDigits(parts[i])) {
		id.region = strings.ToUpper(parts[i])
		i++
	}
	for i < len(parts) && isVariant(parts[i]) {
		v := strings.ToLower(parts[i])
		if slices.Contains(id.variants, v) {
			return nil, false
		}
		id.variants = append(id.variants, v)
		i++
	}
	seen := map[string]bool{}
	for i < len(parts) {
		single := strings.ToLower(parts[i])
		if len(single) != 1 {
			return nil, false
		}
		if single == "x" {
			if i+1 >= len(parts) {
				return nil, false
			}
			id.private = "x-" + strings.ToLower(strings.Join(parts[i+1:], "-"))
			return id, true
		}
		if seen[single] {
			return nil, false
		}
		seen[single] = true
		j := i + 1
		for j < len(parts) && len(parts[j]) >= 2 {
			j++
		}
		sub := parts[i+1 : j]
		if len(sub) == 0 {
			return nil, false
		}
		switch single {
		case "u":
			if !id.parseUnicodeExtension(sub) {
				return nil, false
			}
		case "t":
			if !validTransform(sub) {
				return nil, false
			}
			id.ext = append(id.ext, "t-"+strings.ToLower(strings.Join(sub, "-")))
		default:
			id.ext = append(id.ext, single+"-"+strings.ToLower(strings.Join(sub, "-")))
		}
		i = j
	}
	return id, true
}

func validTransform(sub []string) bool {
	i := 0
	if i < len(sub) && isASCIIAlpha(sub[i]) && (len(sub[i]) >= 2 && len(sub[i]) <= 3 || len(sub[i]) >= 5) {
		i++
		if i < len(sub) && len(sub[i]) == 4 && isASCIIAlpha(sub[i]) {
			i++
		}
		if i < len(sub) && (len(sub[i]) == 2 && isASCIIAlpha(sub[i]) || len(sub[i]) == 3 && isASCIIDigits(sub[i])) {
			i++
		}
		for i < len(sub) && isVariant(sub[i]) {
			i++
		}
	}
	for i < len(sub) {
		if !(len(sub[i]) == 2 && isTKey(sub[i])) {
			return false
		}
		i++
		n := 0
		for i < len(sub) && len(sub[i]) >= 3 {
			i++
			n++
		}
		if n == 0 {
			return false
		}
	}
	return true
}

func isTKey(s string) bool {
	c := s[0]
	return (c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') && (s[1] >= '0' && s[1] <= '9')
}

func (id *localeID) parseUnicodeExtension(sub []string) bool {
	i := 0
	for i < len(sub) && len(sub[i]) >= 3 {
		id.uAttrs = append(id.uAttrs, strings.ToLower(sub[i]))
		i++
	}
	for i < len(sub) {
		key := strings.ToLower(sub[i])
		if len(key) != 2 || !(key[1] >= 'a' && key[1] <= 'z') {
			return false
		}
		i++
		start := i
		for i < len(sub) && len(sub[i]) >= 3 {
			i++
		}
		value := strings.ToLower(strings.Join(sub[start:i], "-"))
		if value == "true" {
			value = ""
		}
		if !slices.ContainsFunc(id.uKeys, func(k uKeyword) bool { return k.key == key }) {
			id.uKeys = append(id.uKeys, uKeyword{key, value})
		}
	}
	return true
}

func (id *localeID) keyword(key string) (string, bool) {
	for _, k := range id.uKeys {
		if k.key == key {
			return k.value, true
		}
	}
	return "", false
}

// baseName is language-script-region-variants.
func (id *localeID) baseName() string {
	parts := []string{id.lang}
	if id.script != "" {
		parts = append(parts, id.script)
	}
	if id.region != "" {
		parts = append(parts, id.region)
	}
	parts = append(parts, id.variants...)
	return strings.Join(parts, "-")
}

func (id *localeID) String() string {
	s := id.baseName()
	var exts []string
	exts = append(exts, id.ext...)
	if len(id.uAttrs) > 0 || len(id.uKeys) > 0 {
		attrs := append([]string(nil), id.uAttrs...)
		sort.Strings(attrs)
		keys := append([]uKeyword(nil), id.uKeys...)
		sort.SliceStable(keys, func(a, b int) bool { return keys[a].key < keys[b].key })
		u := "u"
		for _, a := range attrs {
			u += "-" + a
		}
		for _, k := range keys {
			u += "-" + k.key
			if k.value != "" {
				u += "-" + k.value
			}
		}
		exts = append(exts, u)
	}
	sort.SliceStable(exts, func(a, b int) bool { return exts[a][0] < exts[b][0] })
	for _, e := range exts {
		s += "-" + e
	}
	if id.private != "" {
		s += "-" + id.private
	}
	return s
}

func aliasTable(kind string) map[string]any {
	t, _ := icudata.Table("supp/aliases")[kind].(map[string]any)
	return t
}

func aliasKey(lang, region, variant string) string {
	k := lang
	if region != "" {
		k += "_" + region
	}
	if variant != "" {
		k += "_" + variant
	}
	return k
}

// canonicalize applies the alias replacement of UTS #35 Annex C, as ICU's AliasReplacer does.
func (id *localeID) canonicalize() {
	sort.Strings(id.variants)
	languages := aliasTable("language")
	territories := aliasTable("territory")
	scripts := aliasTable("script")
	variantTable := aliasTable("variant")

	replaceLanguage := func(checkLang, checkRegion, checkVariants bool) bool {
		if checkRegion && id.region == "" || checkVariants && len(id.variants) == 0 {
			return false
		}
		n := 1
		if checkVariants {
			n = len(id.variants)
		}
		searchLang := "und"
		if checkLang {
			searchLang = id.lang
		}
		searchRegion := ""
		if checkRegion {
			searchRegion = id.region
		}
		for i := 0; i < n; i++ {
			searchVariant := ""
			if checkVariants {
				searchVariant = id.variants[i]
				if len(searchVariant) < 4 {
					searchVariant = ""
				}
			}
			repl, _ := languages[aliasKey(searchLang, searchRegion, searchVariant)].(string)
			if repl == "" {
				continue
			}
			rl, rs, rr, rv := parseLanguageReplacement(repl)
			if rl == "und" {
				rl = id.lang
			}
			newScript := id.script
			if rs != "" && id.script == "" {
				newScript = rs
			}
			newRegion := id.region
			if searchRegion != "" {
				newRegion = rr
			} else if rr != "" && id.region == "" {
				newRegion = rr
			}
			newVariant := searchVariant
			if rv != "" {
				newVariant = rv
			} else if searchVariant != "" {
				newVariant = ""
			}
			if rl == id.lang && newScript == id.script && newRegion == id.region && newVariant == searchVariant {
				continue
			}
			id.lang, id.script, id.region = rl, newScript, newRegion
			if searchVariant != "" {
				if newVariant != "" {
					id.variants[i] = newVariant
				} else {
					id.variants = slices.Delete(id.variants, i, i+1)
				}
			}
			sort.Strings(id.variants)
			return true
		}
		return false
	}
	replaceTerritory := func() bool {
		repl, _ := territories[id.region].(string)
		if repl == "" {
			return false
		}
		chosen := repl
		if strings.Contains(repl, " ") {
			lang := id.lang
			likely := maximizeLSR(lang, id.script, "", "")
			options := strings.Fields(repl)
			chosen = options[0]
			if likely[2] != "" && slices.Contains(options, likely[2]) {
				chosen = likely[2]
			}
		}
		if chosen == id.region {
			return false
		}
		id.region = chosen
		return true
	}
	replaceScript := func() bool {
		repl, _ := scripts[id.script].(string)
		if repl == "" || repl == id.script {
			return false
		}
		id.script = repl
		return true
	}
	replaceVariant := func() bool {
		for i, v := range id.variants {
			repl, _ := variantTable[v].(string)
			if repl == "" || repl == v {
				continue
			}
			id.variants[i] = repl
			if v == "heploc" {
				id.variants = slices.DeleteFunc(id.variants, func(s string) bool { return s == "hepburn" })
			}
			sort.Strings(id.variants)
			return true
		}
		return false
	}
	for count := 0; count <= 10; count++ {
		if replaceLanguage(true, true, true) || replaceLanguage(true, true, false) || replaceLanguage(true, false, true) ||
			replaceLanguage(true, false, false) || replaceLanguage(false, false, true) ||
			replaceTerritory() || replaceScript() || replaceVariant() {
			continue
		}
		break
	}
}

func parseLanguageReplacement(repl string) (lang, script, region, variant string) {
	parts := strings.Split(repl, "_")
	lang = parts[0]
	for _, p := range parts[1:] {
		switch {
		case len(p) == 4 && isASCIIAlpha(p):
			script = p
		case len(p) == 2 && isASCIIAlpha(p) || len(p) == 3 && isASCIIDigits(p):
			region = p
		default:
			if variant != "" {
				variant += "_"
			}
			variant += p
		}
	}
	return
}

// canonicalizeTag is CanonicalizeUnicodeLocaleId.
func canonicalizeTag(tag string) *localeID {
	id, ok := parseLocaleTag(tag)
	if !ok {
		rangeError("Incorrect locale information provided")
	}
	id.canonicalize()
	return id
}

var macroRegions = func() map[string]bool {
	m := map[string]bool{"EU": true, "EZ": true, "QO": true, "UN": true}
	for _, r := range strings.Fields("001 002 003 005 009 011 013 014 015 017 018 019 021 029 030 034 035 039 053 054 057 061 142 143 145 150 151 154 155 202 419") {
		m[r] = true
	}
	return m
}()

// maximizeLSR is ICU's LikelySubtags lookup with returnInputIfUnmatch; the empty string is an absent subtag.
func maximizeLSR(language, script, region, variant string) [3]string {
	likely := icudata.Table("supp/likely")
	langAliases, _ := likely["languageAliases"].(map[string]any)
	regionAliases, _ := likely["regionAliases"].(map[string]any)
	trie, _ := likely["trie"].(map[string]any)
	lsrs, _ := likely["lsrs"].([]any)

	if a, ok := langAliases[language].(string); ok {
		language = a
	}
	if a, ok := regionAliases[region].(string); ok {
		region = a
	}
	inLang, inScript, inRegion := language, script, region
	if language == "und" {
		language = ""
	}
	if script == "Zzzz" {
		script = ""
	}
	if region == "ZZ" {
		region = ""
	}
	if script != "" && region != "" && language != "" {
		return [3]string{language, script, region}
	}
	key := func(s string) string {
		if s == "" {
			return "*"
		}
		return s
	}
	step := func(table map[string]any, k string) (int, map[string]any) {
		e, ok := table[key(k)].(map[string]any)
		if !ok {
			return -1, nil
		}
		next, _ := e["n"].(map[string]any)
		if v, ok := e["v"].(float64); ok {
			return int(v), next
		}
		return 0, next
	}
	und, _ := trie["*"].(map[string]any)
	undNext, _ := und["n"].(map[string]any)
	value, next := step(trie, language)
	matchLanguage := value >= 0
	var retainLanguage, retainScript, retainRegion bool
	var state, cur map[string]any
	if value >= 0 {
		retainLanguage = language != ""
		state, cur = next, next
	} else {
		retainLanguage = true
		cur = undNext
	}
	matchScript := false
	if value > 0 {
		if value == 1 {
			value = 0
		}
		retainScript = script != ""
	} else {
		var nextScript map[string]any
		value, nextScript = step(cur, script)
		matchScript = value >= 0 && script != ""
		if value >= 0 {
			retainScript = script != ""
			state, cur = nextScript, nextScript
		} else {
			retainScript = true
			if state == nil {
				_, cur = step(undNext, "")
			} else {
				value, nextScript = step(state, "")
				state, cur = nextScript, nextScript
			}
		}
	}
	matchRegion := false
	if value > 0 {
		retainRegion = region != ""
	} else {
		value, _ = step(cur, region)
		matchRegion = value >= 0
		if value >= 0 {
			if region != "" && !macroRegions[region] {
				retainRegion = true
			}
		} else {
			retainRegion = region != ""
			if state != nil {
				value, _ = step(state, "")
			}
			if state == nil || value < 0 {
				// No entry for this script: the region alone may still name a language.
				star, _ := undNext["*"].(map[string]any)
				regions, _ := star["n"].(map[string]any)
				value, _ = step(regions, region)
			}
			if value < 0 {
				value = defaultLSRIndex(undNext)
			}
		}
	}
	if !(matchLanguage || matchScript || (matchRegion && region != "")) {
		return [3]string{inLang, inScript, inRegion}
	}
	m := lsrs[value].([]any)
	if language == "" {
		language = "und"
	}
	if !(retainLanguage || retainScript || retainRegion) {
		return [3]string{m[0].(string), m[1].(string), m[2].(string)}
	}
	if !retainLanguage {
		language = m[0].(string)
	}
	if !retainScript {
		script = m[1].(string)
	}
	if !retainRegion {
		region = m[2].(string)
	}
	return [3]string{language, script, region}
}

// defaultLSRIndex is the trie's entry for an unknown language, script and region.
func defaultLSRIndex(undNext map[string]any) int {
	e, _ := undNext["*"].(map[string]any)
	n, _ := e["n"].(map[string]any)
	leaf, _ := n["*"].(map[string]any)
	v, _ := leaf["v"].(float64)
	return int(v)
}

// Locale is Intl.Locale.
type Locale struct{ id *localeID }

// NewLocale is new Intl.Locale(tag); an invalid tag raises RangeError.
func NewLocale(tag string) *Locale {
	if tag == "" {
		rangeError("First argument to Intl.Locale constructor can't be empty or missing")
	}
	return &Locale{id: canonicalizeTag(tag)}
}

// NewLocaleWithScript is new Intl.Locale(language, {script}); an empty script is undefined.
func NewLocaleWithScript(language, script string) *Locale {
	id := canonicalizeTag(language)
	if script != "" {
		id.script = script
		id.canonicalize()
	}
	return &Locale{id: id}
}

// Language is the language getter.
func (l *Locale) Language() string {
	if l.id.lang == "und" {
		return ""
	}
	return l.id.lang
}

// Script is the script getter; "" is undefined.
func (l *Locale) Script() string { return l.id.script }

// Region is the region getter; "" is undefined.
func (l *Locale) Region() string { return l.id.region }

// Maximize is maximize().
func (l *Locale) Maximize() *Locale {
	variant := ""
	if len(l.id.variants) > 0 {
		variant = l.id.variants[0]
	}
	r := maximizeLSR(l.id.lang, l.id.script, l.id.region, variant)
	out := *l.id
	out.lang = r[0]
	if out.lang == "" {
		out.lang = "und"
	}
	out.script, out.region = r[1], r[2]
	if len(icuBaseName(&out)) == len(icuBaseName(l.id)) {
		// V8 keeps the locale when the base name's length did not change; ICU's base name has no "und".
		return l
	}
	return &Locale{id: &out}
}

func icuBaseName(id *localeID) string {
	base := id.baseName()
	if id.lang == "und" {
		return base[3:]
	}
	return base
}

// resolved is the outcome of ResolveLocale for one service.
type resolved struct {
	tag     string // the matched available locale, without extensions
	icuName string // tag as an ICU bundle name
	id      *localeID
	req     *localeID // the requested locale the match came from, with its extensions; nil for the default
}

func availableSet(service string) map[string]bool {
	cached, ok := availableCache[service]
	if ok {
		return cached
	}
	idx := icudata.Table("supp/index")
	avail, _ := idx["available"].(map[string]any)
	list, _ := avail[service].([]any)
	m := make(map[string]bool, len(list))
	for _, v := range list {
		m[v.(string)] = true
	}
	availableCache[service] = m
	return m
}

var availableCache = map[string]map[string]bool{}

func bestAvailable(avail map[string]bool, tag string) string {
	candidate := tag
	for {
		if avail[candidate] {
			return candidate
		}
		pos := strings.LastIndexByte(candidate, '-')
		if pos < 0 {
			return ""
		}
		if pos >= 2 && candidate[pos-2] == '-' {
			pos -= 2
		}
		candidate = candidate[:pos]
	}
}

const defaultLocale = "en-US"

// resolveLocale is CanonicalizeLocaleList followed by the lookup matcher.
func resolveLocale(locales []string, service string) resolved {
	avail := availableSet(service)
	canon := make([]*localeID, 0, len(locales))
	for _, tag := range locales {
		canon = append(canon, canonicalizeTag(tag))
	}
	for _, id := range canon {
		base := id.baseName()
		if match := bestAvailable(avail, base); match != "" {
			mid, _ := parseLocaleTag(match)
			return resolved{tag: match, icuName: strings.ReplaceAll(match, "-", "_"), id: mid, req: id}
		}
	}
	mid, _ := parseLocaleTag(defaultLocale)
	return resolved{tag: defaultLocale, icuName: "en_US", id: mid}
}

func (r resolved) keyword(key string) string {
	if r.req == nil {
		return ""
	}
	v, _ := r.req.keyword(key)
	return v
}
