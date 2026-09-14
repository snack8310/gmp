package experimentation

import "sync"

// ColoringRecord is one assignment frozen in place.
//
// It carries the algorithm version that produced it because replaying has to
// use the algorithm that actually ran. Recomputing an old assignment under a
// newer algorithm disagrees with what happened without failing.
type ColoringRecord struct {
	Bucket    BucketNumber
	Algorithm AlgorithmVersion
}

// ColoringStore keeps assignments that must survive a change of definition.
//
// Records are filed under the assignment id, not under a definition value, so
// that building a new definition for the same assignment leaves the already
// coloured where they are.
//
// Implementations must make RecordFirst atomic per key. The in-memory
// implementation cannot demonstrate a race on its own, so the guarantee lives
// in this contract and is carried by whatever implements it.
type ColoringStore interface {
	// Lookup returns the record for this key, if one was ever written.
	Lookup(id AssignmentID, key AssignmentKey) (ColoringRecord, bool, error)
	// RecordFirst writes the record only if this key has none, and returns
	// whichever record is in force afterwards. Concurrent callers for one key
	// must all be told the same winner.
	RecordFirst(id AssignmentID, key AssignmentKey, record ColoringRecord) (ColoringRecord, error)
}

type coloringSlot struct {
	id  AssignmentID
	key AssignmentKey
}

// MemoryColoringStore is the in-memory ColoringStore: the first implementation,
// deliberately not backed by a database.
//
// It exists so the contract tests are free to write now rather than after a
// real store lands, which is the only point at which they are cheap.
type MemoryColoringStore struct {
	mu      sync.Mutex
	records map[coloringSlot]ColoringRecord
}

// NewMemoryColoringStore builds an empty store.
func NewMemoryColoringStore() *MemoryColoringStore {
	return &MemoryColoringStore{records: make(map[coloringSlot]ColoringRecord)}
}

// Lookup implements ColoringStore.
func (s *MemoryColoringStore) Lookup(id AssignmentID, key AssignmentKey) (ColoringRecord, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.records[coloringSlot{id: id, key: key}]
	return rec, ok, nil
}

// RecordFirst implements ColoringStore. The whole read-decide-write runs under
// one lock, so a second caller for the same key observes the first one's
// record rather than overwriting it.
func (s *MemoryColoringStore) RecordFirst(id AssignmentID, key AssignmentKey, record ColoringRecord) (ColoringRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	slot := coloringSlot{id: id, key: key}
	if existing, ok := s.records[slot]; ok {
		return existing, nil
	}
	s.records[slot] = record
	return record, nil
}

// Size reports how many records are held. Tests use it to show that a dry run
// wrote nothing at all, which asserting on a single key could not.
func (s *MemoryColoringStore) Size() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.records)
}
