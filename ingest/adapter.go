package ingest

import (
	"io"
	"slices"
	"sync"
)

// Adapter reads the snapshots of one source. Each source in spec 09.2 has one adapter, in the package ingest/<source>. The package registers the adapter in an init function, and the m2 command imports the package.
type Adapter interface {
	// Source returns the catalogue entry of the source, without snapshots. The first import of a source adds this entry to the catalogue. After that, the entry in the catalogue is used, because a maintainer approves the class there.
	Source() Source

	// Version names the adapter and its version, for example oeis/1. A change to the records that the adapter gives needs a new version.
	Version() string

	// Dump returns the URL of the bulk channel of the source.
	Dump() string

	// Read reads one snapshot and calls emit for each item. Read returns the first error from emit.
	Read(snapshot io.Reader, emit func(Record) error) error
}

var (
	mu       sync.RWMutex
	adapters = map[string]Adapter{}
)

// Register makes an adapter available by the id of its source. Register panics when the id has an adapter already.
func Register(a Adapter) {
	mu.Lock()
	defer mu.Unlock()
	id := a.Source().ID
	if _, dup := adapters[id]; dup {
		panic("ingest: Register called twice for the source " + id)
	}
	adapters[id] = a
}

// Lookup returns the adapter of the source with the id.
func Lookup(id string) (Adapter, bool) {
	mu.RLock()
	defer mu.RUnlock()
	a, ok := adapters[id]
	return a, ok
}

// Adapters returns the ids of the sources that have an adapter, in order.
func Adapters() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(adapters))
	for id := range adapters {
		out = append(out, id)
	}
	slices.Sort(out)
	return out
}
