// Ported from fluent.js (Apache-2.0), fluent-bundle/src/bundle.ts.

// Package bundle is the Fluent runtime: a port of @fluent/bundle. A FluentBundle is a
// single-language store of translation resources which formats message values and
// attributes to strings.
//
// An exception of the original is a panic(*fluent.Error) here, with the same name and
// message.
package bundle

import (
	"strings"

	"github.com/projectfluent/fluent-go"
	"github.com/projectfluent/fluent-go/internal/intl"
)

// typeError builds a TypeError.
func typeError(message string) *fluent.Error {
	return &fluent.Error{Name: "TypeError", Message: message}
}

// FluentFunction is a function available to translations. It raises an exception with
// panic(*fluent.Error).
type FluentFunction = func(positional []FluentValue, named map[string]FluentValue) FluentValue

// TextTransform is a function applied to the text parts of patterns.
type TextTransform = func(string) string

// Options configures a FluentBundle.
type Options struct {
	// Functions are available to translations next to, or instead of, NUMBER and
	// DATETIME. A nil entry leaves the name unknown.
	Functions map[string]FluentFunction
	// UseIsolating says whether to wrap placeables in bidi isolation marks, which they
	// only get in patterns of more than one element. nil means the default, true.
	UseIsolating *bool
	// Transform is applied to text. nil means the default, the identity.
	Transform TextTransform
}

// AddResourceOptions configures FluentBundle.AddResource.
type AddResourceOptions struct{ AllowOverrides bool }

// FluentBundle is a single-language store of messages and terms. It memoizes its Intl
// formatters while formatting, so it is not safe for concurrent use.
type FluentBundle struct {
	locales      []string
	terms        map[string]*Message
	messages     map[string]*Message
	functions    map[string]FluentFunction
	useIsolating bool
	transform    TextTransform

	numberFormats   map[NumberOptions]*intl.NumberFormat
	pluralRules     map[NumberOptions]*intl.PluralRules
	dateTimeFormats map[DateTimeOptions]*intl.DateTimeFormat
}

// NewFluentBundle returns an empty bundle. The locales are used to instantiate the Intl
// formatters translations need.
func NewFluentBundle(locales []string, opts Options) *FluentBundle {
	b := &FluentBundle{
		locales:         locales,
		terms:           map[string]*Message{},
		messages:        map[string]*Message{},
		functions:       builtins(opts.Functions),
		useIsolating:    opts.UseIsolating == nil || *opts.UseIsolating,
		transform:       opts.Transform,
		numberFormats:   map[NumberOptions]*intl.NumberFormat{},
		pluralRules:     map[NumberOptions]*intl.PluralRules{},
		dateTimeFormats: map[DateTimeOptions]*intl.DateTimeFormat{},
	}
	if b.transform == nil {
		b.transform = func(text string) string { return text }
	}
	return b
}

// Locales returns the bundle's locales.
func (b *FluentBundle) Locales() []string { return b.locales }

// HasMessage reports whether a message is present in the bundle.
func (b *FluentBundle) HasMessage(id string) bool {
	_, ok := b.messages[id]
	return ok
}

// GetMessage returns a raw message, or nil if it is not in the bundle.
func (b *FluentBundle) GetMessage(id string) *Message { return b.messages[id] }

// AddResource adds the messages and terms of a resource. Unless overrides are allowed,
// an entry that already exists is left alone and an Error is returned for it.
func (b *FluentBundle) AddResource(res *FluentResource, opts AddResourceOptions) []error {
	errs := []error{}
	for _, entry := range res.body {
		store, kind := b.messages, "message"
		if strings.HasPrefix(entry.ID, "-") {
			store, kind = b.terms, "term"
		}
		if _, exists := store[entry.ID]; exists && !opts.AllowOverrides {
			errs = append(errs, &fluent.Error{
				Name:    "Error",
				Message: "Attempt to override an existing " + kind + `: "` + entry.ID + `"`,
			})
			continue
		}
		store[entry.ID] = entry
	}
	return errs
}

// FormatPattern formats a pattern to a string, using args to resolve references to
// variables. A nil args is null; any other map stands for a plain JavaScript object, with
// Object.prototype behind it. A variable's value is a string, float64, JSDate, FluentType,
// Undefined, nil (null) or any other Go value, which stands for a JavaScript object.
//
// Errors are appended to errs and as much of the translation as possible is salvaged. A
// nil errs is null, so the first error panics instead.
func (b *FluentBundle) FormatPattern(pattern Pattern, args map[string]any, errs *[]error) (result string) {
	// A simple pattern needs no scope and no error handling.
	if text, ok := pattern.(string); ok {
		return b.transform(text)
	}

	scope := newScope(b, errs, args)
	defer func() {
		if r := recover(); r != nil {
			err, ok := r.(*fluent.Error)
			if errs == nil || !ok {
				panic(r)
			}
			*errs = append(*errs, err)
			result = NewFluentNone().ToString(scope)
		}
	}()
	return valueToString(resolveComplexPattern(scope, pattern), scope)
}
