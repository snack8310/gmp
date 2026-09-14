package audience

import "sort"

// MemoryFeed is an in-memory Feed: the first implementation, deliberately not
// backed by anything real.
type MemoryFeed struct {
	records map[UID]Record
}

// NewMemoryFeed builds a feed holding the given records.
func NewMemoryFeed(records ...Record) *MemoryFeed {
	held := make(map[UID]Record, len(records))
	for _, record := range records {
		held[record.UID] = record
	}
	return &MemoryFeed{records: held}
}

// All implements Feed. Records come back in a stable order so that an
// enumeration is reproducible.
func (f *MemoryFeed) All() ([]Record, error) {
	out := make([]Record, 0, len(f.records))
	for _, record := range f.records {
		out = append(out, record)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UID < out[j].UID })
	return out, nil
}

// Lookup implements Feed.
func (f *MemoryFeed) Lookup(uid UID) (Record, bool, error) {
	record, ok := f.records[uid]
	return record, ok, nil
}
