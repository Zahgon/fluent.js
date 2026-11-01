// Ported from fluent.js (Apache-2.0), fluent-sequence/src/map_sync.ts.

// Package sequence maps message identifiers to the best bundle of a lazily produced
// sequence: a port of the synchronous part of @fluent/sequence.
package sequence

import (
	"iter"

	"github.com/projectfluent/fluent-go/bundle"
)

// MapBundleSync returns the first bundle with a message for id, or nil. It stops pulling
// bundles as soon as it has found one.
func MapBundleSync(bundles iter.Seq[*bundle.FluentBundle], id string) *bundle.FluentBundle {
	for candidate := range bundles {
		if candidate.HasMessage(id) {
			return candidate
		}
	}
	return nil
}

// MapBundleSyncAll returns the first bundle with a message for each of ids, or nil for
// that id. Each id restarts the iteration.
func MapBundleSyncAll(bundles iter.Seq[*bundle.FluentBundle], ids []string) []*bundle.FluentBundle {
	found := make([]*bundle.FluentBundle, len(ids))
	for i, id := range ids {
		found[i] = MapBundleSync(bundles, id)
	}
	return found
}
