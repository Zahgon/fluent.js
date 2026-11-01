// Package icudata holds the CLDR and time zone data of ICU 78.3 (CLDR 48.0, tz 2026e), exported from the
// pinned Node.js binary by gen/mkicu.py. See THIRD_PARTY_NOTICES.md for the Unicode licence of the data.
package icudata

import (
	"archive/zip"
	"bytes"
	_ "embed" // the data archive
	"encoding/json"
	"io"
	"sync"
)

//go:embed icu.zip
var archive []byte

var (
	once    sync.Once
	files   map[string]*zip.File
	mu      sync.Mutex
	decoded = map[string]any{}
)

func open() {
	r, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		panic("icudata: corrupt archive: " + err.Error())
	}
	files = make(map[string]*zip.File, len(r.File))
	for _, f := range r.File {
		files[f.Name] = f
	}
}

// Has reports whether the archive holds the entry.
func Has(name string) bool {
	once.Do(open)
	_, ok := files[name]
	return ok
}

// Get returns the decoded JSON of an entry, or nil when the archive does not hold it. The result is shared
// and must not be modified.
func Get(name string) any {
	once.Do(open)
	mu.Lock()
	defer mu.Unlock()
	if v, ok := decoded[name]; ok {
		return v
	}
	f, ok := files[name]
	if !ok {
		decoded[name] = nil
		return nil
	}
	rc, err := f.Open()
	if err != nil {
		panic("icudata: " + name + ": " + err.Error())
	}
	defer rc.Close()
	raw, err := io.ReadAll(rc)
	if err != nil {
		panic("icudata: " + name + ": " + err.Error())
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		panic("icudata: " + name + ": " + err.Error())
	}
	decoded[name] = v
	return v
}

// Table returns an entry that is a JSON object; nil when absent.
func Table(name string) map[string]any {
	t, _ := Get(name).(map[string]any)
	return t
}
