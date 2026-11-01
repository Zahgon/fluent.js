package sequence

import (
	"iter"
	"testing"

	"github.com/projectfluent/fluent-go/bundle"
)

func newBundle(source string) *bundle.FluentBundle {
	b := bundle.NewFluentBundle([]string{"en-US"}, bundle.Options{})
	b.AddResource(bundle.NewFluentResource(source), bundle.AddResourceOptions{})
	return b
}

// counted returns a sequence over bundles and the number of bundles it has yielded.
func counted(bundles ...*bundle.FluentBundle) (iter.Seq[*bundle.FluentBundle], *int) {
	pulled := new(int)
	return func(yield func(*bundle.FluentBundle) bool) {
		for _, b := range bundles {
			*pulled++
			if !yield(b) {
				return
			}
		}
	}, pulled
}

func TestMapBundleSyncStopsAtTheFirstMatch(t *testing.T) {
	first, second, third := newBundle("a = 1"), newBundle("b = 2\na = 3"), newBundle("c = 4")
	bundles, pulled := counted(first, second, third)

	if got := MapBundleSync(bundles, "b"); got != second {
		t.Errorf("MapBundleSync(b) = %p, want the second bundle %p", got, second)
	}
	if *pulled != 2 {
		t.Errorf("pulled %d bundles, want 2", *pulled)
	}

	*pulled = 0
	if got := MapBundleSync(bundles, "missing"); got != nil {
		t.Errorf("MapBundleSync(missing) = %p, want nil", got)
	}
	if *pulled != 3 {
		t.Errorf("pulled %d bundles, want 3", *pulled)
	}
}

func TestTermsAreNotMessages(t *testing.T) {
	bundles, _ := counted(newBundle("-a = 1"))
	if got := MapBundleSync(bundles, "-a"); got != nil {
		t.Errorf("MapBundleSync(-a) = %p, want nil", got)
	}
}

func TestMapBundleSyncAllRestartsForEachID(t *testing.T) {
	first, second := newBundle("a = 1"), newBundle("b = 2")
	bundles, pulled := counted(first, second)

	got := MapBundleSyncAll(bundles, []string{"b", "a", "c"})
	want := []*bundle.FluentBundle{second, first, nil}
	if len(got) != len(want) {
		t.Fatalf("MapBundleSyncAll returned %d bundles, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("bundle %d = %p, want %p", i, got[i], want[i])
		}
	}
	if *pulled != 5 {
		t.Errorf("pulled %d bundles, want 2 + 1 + 2", *pulled)
	}

	if got := MapBundleSyncAll(bundles, nil); got == nil || len(got) != 0 {
		t.Errorf("MapBundleSyncAll with no ids = %#v, want an empty slice", got)
	}
}
