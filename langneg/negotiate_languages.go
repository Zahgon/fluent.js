// Ported from fluent.js (Apache-2.0), fluent-langneg/src/negotiate_languages.ts.

// Package langneg negotiates between requested and available locales: a port of
// @fluent/langneg.
package langneg

import (
	"slices"

	"github.com/projectfluent/fluent-go"
)

// Options configures NegotiateLanguages.
type Options struct {
	// Strategy is "filtering", "matching" or "lookup". "" means the default, filtering.
	Strategy string
	// DefaultLocale is the last-resort locale. nil is undefined; lookup requires it.
	DefaultLocale *string
}

// NegotiateLanguages returns the available locales to use, in the order of the user's
// preferences expressed by requested. The strategy "lookup" without a default locale
// raises Error.
func NegotiateLanguages(requested, available []string, opts Options) []string {
	strategy := opts.Strategy
	if strategy == "" {
		strategy = "filtering"
	}
	supported := FilterMatches(requested, available, strategy)

	if strategy == "lookup" {
		if opts.DefaultLocale == nil {
			panic(&fluent.Error{
				Name:    "Error",
				Message: "defaultLocale cannot be undefined for strategy `lookup`",
			})
		}
		if len(supported) == 0 {
			supported = append(supported, *opts.DefaultLocale)
		}
	} else if opts.DefaultLocale != nil && *opts.DefaultLocale != "" && !slices.Contains(supported, *opts.DefaultLocale) {
		supported = append(supported, *opts.DefaultLocale)
	}
	return supported
}
