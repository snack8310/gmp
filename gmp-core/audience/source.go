package audience

import (
	"errors"
	"fmt"
	"time"
)

var (
	// ErrMissingSourceID is returned when a source carries no identity.
	ErrMissingSourceID = errors.New("audience: source id is empty")
	// ErrUnknownSupply is returned when a source does not say how it hands
	// over ids. There is no default: the three ways differ in who acts.
	ErrUnknownSupply = errors.New("audience: source supply is not one of the known ways")
	// ErrUnknownCategory is returned when a source does not say whether it is
	// external or this platform's own execution results flowing back.
	ErrUnknownCategory = errors.New("audience: source category is not one of the known kinds")
	// ErrGranularityNotDeclared is returned when a condition needs to know how
	// far behind a source runs and the source never said.
	//
	// The documents say hour-level is good enough for this platform's own
	// backflow. Writing that in as a constant would be inventing a default the
	// repository cannot prove for any particular source, so a source that wants
	// to carry time-window conditions has to state its own.
	ErrGranularityNotDeclared = errors.New("audience: source did not declare its backflow granularity")
	// ErrNonPositiveGranularity is returned when a declared granularity is not
	// a real length of time.
	ErrNonPositiveGranularity = errors.New("audience: declared backflow granularity is not positive")
)

// Supply is how a source hands ids over. All three are the same thing seen
// from different sides -- somewhere that yields ids -- and differ only in who
// acts first.
type Supply string

const (
	// SupplyPull is asked for ids.
	SupplyPull Supply = "pull"
	// SupplyPushBatch hands over a batch.
	SupplyPushBatch Supply = "push-batch"
	// SupplyPushStream hands over one at a time.
	SupplyPushStream Supply = "push-stream"
)

func (s Supply) validate() error {
	switch s {
	case SupplyPull, SupplyPushBatch, SupplyPushStream:
		return nil
	default:
		return fmt.Errorf("%w: %q", ErrUnknownSupply, string(s))
	}
}

// Category separates outside systems from this platform's own execution
// results flowing back. The second kind is what makes routing on an earlier
// step's outcome work at all.
type Category string

const (
	// CategoryExternal is an outside system.
	CategoryExternal Category = "external"
	// CategoryBackflow is this platform's own execution results returning.
	CategoryBackflow Category = "backflow"
)

func (c Category) validate() error {
	switch c {
	case CategoryExternal, CategoryBackflow:
		return nil
	default:
		return fmt.Errorf("%w: %q", ErrUnknownCategory, string(c))
	}
}

// SourceID identifies a registered source.
type SourceID string

// Source describes somewhere ids come from. Engineering registers these;
// operations picks from the list and cannot wire up a new one.
//
// Values are immutable once built.
type Source struct {
	id          SourceID
	supply      Supply
	category    Category
	granularity time.Duration
	declared    bool
}

// SourceSpec is the input to registering a source.
type SourceSpec struct {
	// ID identifies the source.
	ID SourceID
	// Supply is how it hands ids over.
	Supply Supply
	// Category says whether it is external or this platform's own backflow.
	Category Category
	// BackflowGranularity is how far behind this source runs: the shortest
	// window a condition over it can honestly ask about. Leave it unset when
	// the source has no such property; conditions needing it will then refuse
	// to be built rather than assume one.
	BackflowGranularity time.Duration
}

// RegisterSource builds a source descriptor.
func RegisterSource(spec SourceSpec) (Source, error) {
	if spec.ID == "" {
		return Source{}, ErrMissingSourceID
	}
	if err := spec.Supply.validate(); err != nil {
		return Source{}, fmt.Errorf("%w (source %q)", err, spec.ID)
	}
	if err := spec.Category.validate(); err != nil {
		return Source{}, fmt.Errorf("%w (source %q)", err, spec.ID)
	}
	declared := spec.BackflowGranularity != 0
	if declared && spec.BackflowGranularity <= 0 {
		return Source{}, fmt.Errorf("%w: source %q declared %s", ErrNonPositiveGranularity, spec.ID, spec.BackflowGranularity)
	}
	return Source{
		id:          spec.ID,
		supply:      spec.Supply,
		category:    spec.Category,
		granularity: spec.BackflowGranularity,
		declared:    declared,
	}, nil
}

// ID reports which source this is.
func (s Source) ID() SourceID { return s.id }

// Supply reports how this source hands ids over.
func (s Source) Supply() Supply { return s.supply }

// Category reports whether this source is external or this platform's own.
func (s Source) Category() Category { return s.category }

// BackflowGranularity reports how far behind this source runs, and whether it
// ever said.
func (s Source) BackflowGranularity() (time.Duration, bool) {
	return s.granularity, s.declared
}

func (s Source) isZero() bool { return s.id == "" }

// Record is what a source knows about one person right now.
//
// Event ages come from the source, which is outside this model and has a
// clock. Nothing in this package asks what time it is; it only compares the
// age a source reports against a window someone configured.
type Record struct {
	UID UID
	// Attributes are the facts this source reports, by name.
	Attributes map[string]string
	// EventAges is how long ago each named event happened, as the source
	// observed it. An event that never happened is absent.
	EventAges map[string]time.Duration
}

// Feed hands over what a source actually holds.
//
// The two methods are the two directions this layer exists to serve, and they
// are deliberately separate: enumeration may run offline over everything,
// while a lookup happens with a caller waiting. Implementations must answer
// consistently -- a uid Lookup finds must be one All yields.
type Feed interface {
	// All yields every record this source currently holds.
	All() ([]Record, error)
	// Lookup finds one record without walking the rest.
	Lookup(uid UID) (Record, bool, error)
}
