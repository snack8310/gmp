package scenarios

import (
	"fmt"
	"sync"
	"time"

	"github.com/snack8310/gmp/gmp-core/audience"
	"github.com/snack8310/gmp/gmp-core/campaign"
)

const (
	// BackflowSourceID is the source this platform's own results flow back
	// into. Results returning is what makes routing on an earlier step's
	// outcome possible at all -- a source is no longer only an outside system.
	BackflowSourceID = audience.SourceID("platform-backflow")
	// BackflowGranularity is how far behind this source runs. The documents
	// settle hour-level as good enough for this platform's own backflow, so a
	// condition asking about anything shorter is refused rather than answered
	// with "nobody opened it" for everyone.
	BackflowGranularity = time.Hour
)

// Backflow is where what this platform did comes back as something it can act
// on: it takes events on one side and answers as an audience source on the
// other.
//
// It sits here rather than in either layer because it is the only place both
// are visible. The audience layer must not know what a campaign is -- results
// returning are an ordinary source to it, the same kind of thing as an order
// system.
//
// It owns the clock, and deliberately so. The core has none: waiting until a
// moment arrives belongs to whatever schedules work, and a source reporting how
// long ago something happened is how time enters the model at all. Advance is
// what lets "four hours later" be a step in a test rather than a wait.
type Backflow struct {
	mu      sync.Mutex
	elapsed time.Duration
	records map[audience.UID]*backflowRecord
}

type backflowRecord struct {
	attributes map[string]string
	happenedAt map[string]time.Duration
}

// NewBackflow builds an empty backflow source.
func NewBackflow() *Backflow {
	return &Backflow{records: make(map[audience.UID]*backflowRecord)}
}

// Source describes this backflow for registration with an audience service.
func (b *Backflow) Source() (audience.Source, error) {
	return audience.RegisterSource(audience.SourceSpec{
		ID:                  BackflowSourceID,
		Supply:              audience.SupplyPushStream,
		Category:            audience.CategoryBackflow,
		BackflowGranularity: BackflowGranularity,
	})
}

// Publish implements campaign.EventSink.
//
// An event becomes two things about the person: that it happened at all, and
// whatever the channel said alongside it. Channel vocabulary is namespaced by
// its action, because one channel's "read" is not another's.
func (b *Backflow) Publish(event campaign.Event) error {
	if event.UID == "" {
		return fmt.Errorf("scenarios: backflow got an event with no uid: %q", event.Name)
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	record, held := b.records[event.UID]
	if !held {
		record = &backflowRecord{
			attributes: make(map[string]string),
			happenedAt: make(map[string]time.Duration),
		}
		b.records[event.UID] = record
	}
	name := string(event.Name)
	record.attributes[name] = "yes"
	record.happenedAt[name] = b.elapsed
	for key, value := range event.Attributes {
		record.attributes[string(event.Action)+"."+key] = value
	}
	return nil
}

// Advance moves this source's notion of how long ago things happened.
//
// Nothing in the model asks what time it is; what a condition compares against
// is the age this source reports. Moving it is how a scenario says "and then
// four hours passed" without anything having to wait.
func (b *Backflow) Advance(d time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.elapsed += d
}

// All implements audience.Feed.
func (b *Backflow) All() ([]audience.Record, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]audience.Record, 0, len(b.records))
	for uid := range b.records {
		out = append(out, b.recordFor(uid))
	}
	sortRecords(out)
	return out, nil
}

// Lookup implements audience.Feed.
func (b *Backflow) Lookup(uid audience.UID) (audience.Record, bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, held := b.records[uid]; !held {
		return audience.Record{}, false, nil
	}
	return b.recordFor(uid), true, nil
}

// Size reports how many people this source knows about.
func (b *Backflow) Size() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.records)
}

// recordFor renders one person as this source currently sees them. The caller
// holds the lock.
func (b *Backflow) recordFor(uid audience.UID) audience.Record {
	held := b.records[uid]
	attributes := make(map[string]string, len(held.attributes))
	for key, value := range held.attributes {
		attributes[key] = value
	}
	ages := make(map[string]time.Duration, len(held.happenedAt))
	for name, at := range held.happenedAt {
		ages[name] = b.elapsed - at
	}
	return audience.Record{UID: uid, Attributes: attributes, EventAges: ages}
}
