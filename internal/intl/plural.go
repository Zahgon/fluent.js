package intl

import (
	"math"
	"strconv"
	"strings"

	"github.com/projectfluent/fluent-go/internal/icudata"
)

// operands are the plural operands of UTS #35: n i v w f t c.
type operands struct {
	n    float64
	i    uint64
	v, w int
	f, t uint64
	e    int64
}

func (o operands) get(name byte) float64 {
	switch name {
	case 'n':
		return o.n
	case 'i':
		return float64(o.i)
	case 'v':
		return float64(o.v)
	case 'w':
		return float64(o.w)
	case 'f':
		return float64(o.f)
	case 't':
		return float64(o.t)
	case 'c', 'e':
		return float64(o.e)
	}
	return 0
}

type pluralRule struct {
	category string
	or       [][]pluralRelation // or of ands
}

type pluralRelation struct {
	operand byte
	mod     int
	negate  bool
	ranges  [][2]float64
}

func parseRuleText(category, text string) pluralRule {
	rule := pluralRule{category: category}
	if text == "" {
		return rule
	}
	for _, orPart := range strings.Split(text, " or ") {
		var ands []pluralRelation
		for _, andPart := range strings.Split(orPart, " and ") {
			ands = append(ands, parseRelation(strings.TrimSpace(andPart)))
		}
		rule.or = append(rule.or, ands)
	}
	return rule
}

func parseRelation(s string) pluralRelation {
	rel := pluralRelation{operand: s[0]}
	rest := strings.TrimSpace(s[1:])
	if strings.HasPrefix(rest, "%") {
		rest = strings.TrimSpace(rest[1:])
		end := 0
		for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
			end++
		}
		rel.mod, _ = strconv.Atoi(rest[:end])
		rest = strings.TrimSpace(rest[end:])
	}
	if strings.HasPrefix(rest, "!=") {
		rel.negate = true
		rest = rest[2:]
	} else {
		rest = strings.TrimPrefix(rest, "=")
	}
	for _, item := range strings.Split(strings.TrimSpace(rest), ",") {
		item = strings.TrimSpace(item)
		lo, hi, isRange := strings.Cut(item, "..")
		a, _ := strconv.ParseFloat(lo, 64)
		b := a
		if isRange {
			b, _ = strconv.ParseFloat(hi, 64)
		}
		rel.ranges = append(rel.ranges, [2]float64{a, b})
	}
	return rel
}

func (r pluralRelation) holds(o operands) bool {
	x := o.get(r.operand)
	if r.mod != 0 {
		m := float64(r.mod)
		x = math.Mod(x, m)
	}
	in := false
	for _, rg := range r.ranges {
		if x == float64(int64(x)) && x >= rg[0] && x <= rg[1] {
			in = true
			break
		}
	}
	return in != r.negate
}

func (p pluralRule) matches(o operands) bool {
	for _, ands := range p.or {
		ok := true
		for _, rel := range ands {
			if !rel.holds(o) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// ICU tries a rule set's keywords in its resource table's order, alphabetical: the rules only overlap through the e operand,
// where many wins over one.
var pluralCategoryOrder = []string{"few", "many", "one", "two", "zero"}

var pluralRuleCache = map[string][]pluralRule{}

// pluralRulesFor returns the rule set of an ICU locale name: the nearest ancestor with an entry.
func pluralRulesFor(icuName string, ordinal bool) []pluralRule {
	table := "locales"
	if ordinal {
		table = "ordinals"
	}
	cacheKey := table + "/" + icuName
	if r, ok := pluralRuleCache[cacheKey]; ok {
		return r
	}
	data := icudata.Table("supp/plurals")
	locales, _ := data[table].(map[string]any)
	rules, _ := data["rules"].(map[string]any)
	name := icuName
	var set string
	for {
		if s, ok := locales[name].(string); ok {
			set = s
			break
		}
		i := strings.LastIndexByte(name, '_')
		if i < 0 {
			break
		}
		name = name[:i]
	}
	var out []pluralRule
	if set != "" {
		t, _ := rules[set].(map[string]any)
		for _, cat := range pluralCategoryOrder {
			if text, ok := t[cat].(string); ok {
				out = append(out, parseRuleText(cat, text))
			}
		}
	}
	pluralRuleCache[cacheKey] = out
	return out
}

func selectPlural(rules []pluralRule, o operands) string {
	for _, r := range rules {
		if r.matches(o) {
			return r.category
		}
	}
	return "other"
}

// PluralRules is Intl.PluralRules.
type PluralRules struct {
	rules []pluralRule
	round rounding
}

// NewPluralRules is new Intl.PluralRules(locales, opts). The options are read as the constructor reads them, so
// options of Intl.NumberFormat that it does not know are ignored.
func NewPluralRules(locales []string, opts NumberOptions) *PluralRules {
	r := resolveLocale(locales, "pr")
	stringOption("Intl.PluralRules", "localeMatcher", opts.LocaleMatcher, "lookup", "best fit")
	kind := stringOption("Intl.PluralRules", "type", opts.Type, "cardinal", "ordinal")
	round := digitOptions("Intl.PluralRules", opts, 0, 3, false)
	return &PluralRules{rules: pluralRulesFor(r.icuName, kind == "ordinal"), round: round}
}

// Select is select(value).
func (p *PluralRules) Select(value float64) string {
	d := decimalFromFloat(value)
	if d.nan || d.inf {
		return "other"
	}
	p.round.apply(&d)
	return selectPlural(p.rules, d.operands())
}
