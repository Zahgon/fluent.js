package langneg

import (
	"reflect"
	"testing"

	"github.com/projectfluent/fluent-go"
)

func TestAcceptedLanguages(t *testing.T) {
	tests := []struct {
		header string
		want   []string
	}{
		{"", []string{}},
		{"en", []string{"en"}},
		{" en , de ", []string{"en", "de"}},
		{"en;q=0.5, de, fr;q=0.8", []string{"de", "fr", "en"}},
		{"a;q=0.5,b;q=0.5", []string{"a", "b"}},
		{"a;Q = 0.5,b; q=0.7", []string{"b", "a"}},
		{"a;\tq\u00a0=\u20030.5,b;q=0.7", []string{"b", "a"}},
		{"a;q=abc,b;q=-1,c;q=0", []string{"a", "c", "b"}},
		{"a;q=-0,b;q=0", []string{"a", "b"}},
		{"a;q=Infinity,b", []string{"a", "b"}},
		{"a;q=0.5x,b;q=0.4", []string{"a", "b"}},
		{"a;x=1,b;q=2", []string{"b", "a"}},
		{"a;b,c", []string{"a", "c"}},
		{"a;q=1;junk,b", []string{"a", "b"}},
		{";a,b", []string{"b"}},
		{",,a", []string{"a"}},
		{"  ,a", []string{"", "a"}},
		{"\u00a0é\u2028,😀", []string{"é", "😀"}},
		{"a;q=", []string{"a"}},
		{"a;q", []string{"a"}},
	}
	for _, test := range tests {
		if got := AcceptedLanguages(test.header); !reflect.DeepEqual(got, test.want) {
			t.Errorf("AcceptedLanguages(%q) = %q, want %q", test.header, got, test.want)
		}
	}
}

func TestNegotiateWithoutMatches(t *testing.T) {
	fallback := "fr"
	empty := ""
	tests := []struct {
		name string
		opts Options
		want []string
	}{
		{"no default", Options{}, []string{}},
		{"filtering default", Options{DefaultLocale: &fallback}, []string{"fr"}},
		{"matching default", Options{Strategy: "matching", DefaultLocale: &fallback}, []string{"fr"}},
		{"lookup default", Options{Strategy: "lookup", DefaultLocale: &fallback}, []string{"fr"}},
		{"an empty default is skipped", Options{DefaultLocale: &empty}, []string{}},
		{"an empty lookup default is used", Options{Strategy: "lookup", DefaultLocale: &empty}, []string{""}},
	}
	for _, test := range tests {
		if got := NegotiateLanguages(nil, nil, test.opts); !reflect.DeepEqual(got, test.want) {
			t.Errorf("%s: NegotiateLanguages = %q, want %q", test.name, got, test.want)
		}
	}
}

func TestLookupRequiresDefault(t *testing.T) {
	defer func() {
		want := "defaultLocale cannot be undefined for strategy `lookup`"
		err, ok := recover().(*fluent.Error)
		if !ok || err.Name != "Error" || err.Message != want {
			t.Errorf("raised %v, want Error: %s", err, want)
		}
	}()
	NegotiateLanguages(nil, nil, Options{Strategy: "lookup"})
}

func TestSubstring(t *testing.T) {
	tests := []struct {
		s          string
		start, end int
		want       string
	}{
		{"en-US", 3, 5, "US"},
		{"en-US", 6, 5, ""},
		{"en-US", 9, 9, ""},
		{"en-u-ca-gregory", 3, 2, "-"},
	}
	for _, test := range tests {
		if got := substring(test.s, test.start, test.end); got != test.want {
			t.Errorf("substring(%q, %d, %d) = %q, want %q", test.s, test.start, test.end, got, test.want)
		}
	}
}

func TestExtensionStart(t *testing.T) {
	tests := []struct {
		tag  string
		want int
	}{
		{"en", 2},
		{"en-US", 5},
		{"en-u-ca", 2},
		{"en-US-U-nu", 5},
		{"en-1-x", 6},
		{"en-US-x", 7},
	}
	for _, test := range tests {
		if got := extensionStart(test.tag); got != test.want {
			t.Errorf("extensionStart(%q) = %d, want %d", test.tag, got, test.want)
		}
	}
}

func TestSubtagMatching(t *testing.T) {
	en := subtags{language: "en"}
	enLatnUS := subtags{language: "en", script: "Latn", region: "US"}
	und := subtags{language: "und"}

	tests := []struct {
		name          string
		a, b          subtags
		isRange, want bool
	}{
		{"same", enLatnUS, enLatnUS, false, true},
		{"an available range", en, enLatnUS, false, true},
		{"a requested range needs the flag", enLatnUS, en, false, false},
		{"a requested range with the flag", enLatnUS, en, true, true},
		{"available und", und, enLatnUS, false, true},
		{"requested und", en, und, false, false},
		{"requested und with the flag", en, und, true, true},
		{"different regions", subtags{language: "en", region: "GB"}, enLatnUS, false, false},
	}
	for _, test := range tests {
		if got := test.a.matches(test.b, test.isRange); got != test.want {
			t.Errorf("%s: matches = %v, want %v", test.name, got, test.want)
		}
	}
}
