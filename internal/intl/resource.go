// Package intl reproduces the ECMAScript Intl objects the library needs, as Node.js 24.22.1 implements them with
// ICU 78.3: Intl.NumberFormat, Intl.PluralRules, Intl.DateTimeFormat and Intl.Locale. It reads the CLDR and time zone
// data in internal/icudata through the same resource inheritance ICU uses, and follows ICU's algorithms where they
// decide the output: number skeletons and affixes, date pattern matching in DateTimePatternGenerator, likely subtags,
// alias replacement and time zone name formats.
package intl

import (
	"sort"
	"strings"

	"github.com/projectfluent/fluent-go/internal/icudata"
)

// defaultScript is ICU's getDefaultScript over its compiled table: Latn unless the table says otherwise.
func defaultScript(language, region string) string {
	table := icudata.Table("supp/defaultScripts")
	if region != "" {
		if s, ok := table[language+"_"+region].(string); ok {
			return s
		}
	}
	if s, ok := table[language].(string); ok {
		return s
	}
	return "Latn"
}

// parentLocaleID is ICU's getParentLocaleID: where the search goes from a name with no bundle. "" means stop.
func parentLocaleID(name, origName string) string {
	parts := strings.Split(name, "_")
	language, script, region := parts[0], "", ""
	for _, p := range parts[1:] {
		switch {
		case len(p) == 4 && script == "" && region == "":
			script = p
		case (len(p) == 2 || len(p) == 3) && region == "":
			region = p
		default:
			return chop(name)
		}
	}
	if b := icudata.Table("main/" + name); b != nil {
		if p, ok := b["%%Parent"].(string); ok {
			return p
		}
	}
	switch {
	case script != "" && region != "":
		if defaultScript(language, region) == script {
			return language + "_" + region
		}
		return language + "_" + script
	case region != "":
		origParts := strings.Split(origName, "_")
		if len(origParts) > 1 && len(origParts[1]) == 4 {
			return language + "_" + origParts[1]
		}
		return language + "_" + defaultScript(language, region)
	case script != "":
		if defaultScript(language, "") == script {
			return language
		}
	}
	return ""
}

func chop(name string) string {
	if i := strings.LastIndexByte(name, '_'); i >= 0 {
		return name[:i]
	}
	return ""
}

// chain lists the bundles a lookup in the tree walks through, most specific first: the first existing bundle found
// through parentLocaleID, then its parents by %%Parent or by chopping, root last. %%ALIAS redirects are followed.
func chain(tree, name string) []string {
	key := tree + "/" + name
	if c, ok := chainCache[key]; ok {
		return c
	}
	var out []string
	orig := name
	seen := map[string]bool{}
	cur := name
	found := false
	for cur != "" && !seen[cur] {
		seen[cur] = true
		if b := icudata.Table(tree + "/" + cur); b != nil {
			if a, ok := b["%%ALIAS"].(string); ok {
				cur = a
				orig = a
				continue
			}
			found = true
			break
		}
		cur = parentLocaleID(cur, orig)
	}
	if !found {
		cur = "root"
	}
	seen = map[string]bool{}
	for cur != "" && !seen[cur] {
		seen[cur] = true
		b := icudata.Table(tree + "/" + cur)
		if b != nil {
			if a, ok := b["%%ALIAS"].(string); ok {
				cur = a
				continue
			}
			out = append(out, cur)
			if cur == "root" {
				break
			}
			if p, ok := b["%%Parent"].(string); ok {
				cur = p
				continue
			}
		}
		if next := chop(cur); next != "" {
			cur = next
		} else {
			cur = "root"
		}
	}
	chainCache[key] = out
	return out
}

var chainCache = map[string][]string{}

// walkResult is what one bundle has at a path.
type walkResult struct {
	value any
	alias string // when the path runs through an alias: the alias's target joined with the rest of the path
}

func walk(table map[string]any, path []string) walkResult {
	var cur any = table
	for i, key := range path {
		t, ok := cur.(map[string]any)
		if !ok {
			return walkResult{}
		}
		if a, ok := t["$alias"].(string); ok {
			return walkResult{alias: strings.Join(append([]string{a}, path[i:]...), "/")}
		}
		next, ok := t[key]
		if !ok {
			return walkResult{}
		}
		cur = next
	}
	if t, ok := cur.(map[string]any); ok {
		if a, ok := t["$alias"].(string); ok {
			return walkResult{alias: a}
		}
	}
	return walkResult{value: cur}
}

func walkResultFor(tree, name string, path []string) walkResult {
	return walk(icudata.Table(tree+"/"+name), path)
}

func aliasTarget(alias string) (path []string, ok bool) {
	parts := strings.Split(strings.TrimPrefix(alias, "/"), "/")
	if parts[0] != "LOCALE" {
		return nil, false
	}
	return parts[1:], true
}

// lookup finds a resource by path with inheritance through the locale's chain and through /LOCALE/ aliases.
func lookup(tree, locale string, path ...string) any {
	for depth := 0; depth < 8; depth++ {
		var redirect []string
		for _, name := range chain(tree, locale) {
			r := walk(icudata.Table(tree+"/"+name), path)
			if r.alias != "" {
				if target, ok := aliasTarget(r.alias); ok {
					redirect = target
					break
				}
				return nil
			}
			if r.value != nil {
				return r.value
			}
		}
		if redirect == nil {
			return nil
		}
		path = redirect
	}
	return nil
}

func lookupString(tree, locale string, path ...string) string {
	s, _ := lookup(tree, locale, path...).(string)
	return s
}

func lookupStrings(tree, locale string, path ...string) []string {
	return toStrings(lookup(tree, locale, path...))
}

func toStrings(v any) []string {
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		s, _ := item.(string)
		out = append(out, s)
	}
	return out
}

// merged returns the union of a table over the chain, a more specific bundle's entry winning, following aliases.
func merged(tree, locale string, path ...string) map[string]any {
	out := map[string]any{}
	path = append([]string(nil), path...)
	for depth := 0; depth < 8; depth++ {
		var redirect []string
		for _, name := range chain(tree, locale) {
			r := walk(icudata.Table(tree+"/"+name), path)
			if r.alias != "" {
				if target, ok := aliasTarget(r.alias); ok {
					redirect = target
					break
				}
				return out
			}
			if t, ok := r.value.(map[string]any); ok {
				for k, v := range t {
					if _, have := out[k]; !have {
						out[k] = v
					}
				}
			}
		}
		if redirect == nil {
			return out
		}
		path = redirect
	}
	return out
}

func sortedKeys(t map[string]any) []string {
	keys := make([]string, 0, len(t))
	for k := range t {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
