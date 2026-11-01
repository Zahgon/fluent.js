// Ported from fluent.js (Apache-2.0), fluent-bundle/src/scope.ts.

package bundle

import (
	"github.com/projectfluent/fluent-go"
	"github.com/projectfluent/fluent-go/internal/intl"
)

// Scope is the state of one call to FluentBundle.FormatPattern.
type Scope struct {
	bundle *FluentBundle
	errs   *[]error
	args   map[string]any
	// dirty holds the patterns being resolved, to detect cycles.
	dirty map[*complexPattern]bool
	// params holds the named arguments of the current term reference; nil outside one.
	params map[string]FluentValue
	// placeables is the running count of the placeables resolved so far.
	placeables int
}

func newScope(bundle *FluentBundle, errs *[]error, args map[string]any) *Scope {
	return &Scope{bundle: bundle, errs: errs, args: args, dirty: map[*complexPattern]bool{}}
}

// ReportError collects a *fluent.Error. With no collection, or for any other value, it
// raises err as a panic.
func (s *Scope) ReportError(err any) {
	fluentErr, ok := err.(*fluent.Error)
	if s.errs == nil || !ok {
		panic(err)
	}
	*s.errs = append(*s.errs, fluentErr)
}

// lookupArg is `name in args`, then args[name]. A non-nil map stands for a plain object,
// so it inherits from Object.prototype unless it has an entry of its own.
func (s *Scope) lookupArg(name string) (any, bool) {
	if s.args == nil {
		return nil, false
	}
	if value, ok := s.args[name]; ok {
		return value, true
	}
	if objectPrototype[name] {
		return inheritedFunction, true
	}
	return nil, false
}

// numberFormat returns the bundle's Intl.NumberFormat for the options. A constructor
// that fails stores nothing, so it runs again the next time.
func (s *Scope) numberFormat(opts NumberOptions) *intl.NumberFormat {
	return memoize(s.bundle.numberFormats, opts, func() *intl.NumberFormat {
		return intl.NewNumberFormat(s.bundle.locales, opts)
	})
}

// pluralRules returns the bundle's Intl.PluralRules for the options.
func (s *Scope) pluralRules(opts NumberOptions) *intl.PluralRules {
	return memoize(s.bundle.pluralRules, opts, func() *intl.PluralRules {
		return intl.NewPluralRules(s.bundle.locales, opts)
	})
}

// dateTimeFormat returns the bundle's Intl.DateTimeFormat for the options.
func (s *Scope) dateTimeFormat(opts DateTimeOptions) *intl.DateTimeFormat {
	return memoize(s.bundle.dateTimeFormats, opts, func() *intl.DateTimeFormat {
		return intl.NewDateTimeFormat(s.bundle.locales, opts)
	})
}

// memoize returns the cached value for key, building and storing it if it is absent.
func memoize[K comparable, V any](cache map[K]V, key K, build func() V) V {
	value, ok := cache[key]
	if !ok {
		value = build()
		cache[key] = value
	}
	return value
}
