package campaign

import (
	"errors"
	"fmt"

	"github.com/snack8310/gmp/gmp-core/audience"
)

var (
	// ErrFallbackNotSet is returned when a decision table does not say what
	// happens to whoever matches no row.
	//
	// Leaving it out is how a whole class of people gets dropped with nothing
	// failing, so the table refuses to exist without one -- including the
	// perfectly good answer "nothing".
	ErrFallbackNotSet = errors.New("campaign: decision table has no fallback row")
	// ErrRowNeverReached is returned when a row can be shown to be unreachable:
	// everyone it would match is already taken by an earlier row.
	//
	// Such a row is not a harmless spare. Its numbers stay at zero, and zero
	// reads as "this did not work" rather than "this never ran".
	ErrRowNeverReached = errors.New("campaign: decision table row can never be reached")
	// ErrRowTreatmentNotInCampaign is returned when a row routes somewhere the
	// campaign does not have.
	ErrRowTreatmentNotInCampaign = errors.New("campaign: decision table row routes to a treatment the campaign does not hold")
)

// Row is one line of a decision table: who it catches, and where they go.
type Row struct {
	// When is who this row catches.
	When audience.Definition
	// Then is where they go. The empty id means they go nowhere, which is a
	// decision like any other.
	Then TreatmentID
}

// Fallback is where whoever matched no row goes. It has no usable zero value:
// a table that simply forgot to say would otherwise read as "nowhere", which is
// the same thing an explicit decision looks like.
type Fallback struct {
	then TreatmentID
	set  bool
}

// RouteTo sends the remainder to a treatment.
func RouteTo(id TreatmentID) Fallback { return Fallback{then: id, set: true} }

// RouteNowhere states that the remainder is left alone.
func RouteNowhere() Fallback { return Fallback{set: true} }

// DecisionTable routes people to treatments. It does not run anything itself.
//
// It earns its place by guaranteeing three things a handful of separate
// audience definitions cannot: order (top down, first match wins), a fallback
// (nobody is silently dropped), and exclusivity (only one row is ever taken).
type DecisionTable struct {
	slot     SlotID
	rows     []Row
	fallback Fallback
}

// TableSpec is the input to building a decision table.
type TableSpec struct {
	// Slot is which slot this table routes for.
	Slot SlotID
	// Rows are tried top down; the first match wins.
	Rows []Row
	// Fallback is where the remainder goes.
	Fallback Fallback
}

// NewDecisionTable builds a table, refusing the shapes that fail silently.
func NewDecisionTable(spec TableSpec) (DecisionTable, error) {
	if spec.Slot == "" {
		return DecisionTable{}, ErrMissingSlotID
	}
	if !spec.Fallback.set {
		return DecisionTable{}, fmt.Errorf("%w: slot %q", ErrFallbackNotSet, spec.Slot)
	}
	owned := make([]Row, len(spec.Rows))
	for i, row := range spec.Rows {
		if row.When.ID() == "" {
			return DecisionTable{}, fmt.Errorf("campaign: table for slot %q has an unbuilt condition at row %d", spec.Slot, i+1)
		}
		owned[i] = row
	}
	// A row demanding everything an earlier row demands, and more, can never be
	// reached: the earlier row takes those people first. This is provable, so
	// it is refused. Rows that are unreachable for reasons only meaning can
	// show are not caught here -- see the change record.
	for i := range owned {
		for j := 0; j < i; j++ {
			if audience.Subsumes(owned[j].When, owned[i].When) {
				return DecisionTable{}, fmt.Errorf("%w: slot %q, row %d is taken by row %d first",
					ErrRowNeverReached, spec.Slot, i+1, j+1)
			}
		}
	}
	return DecisionTable{slot: spec.Slot, rows: owned, fallback: spec.Fallback}, nil
}

// Slot reports which slot this table routes for.
func (t DecisionTable) Slot() SlotID { return t.slot }

// Rows reports the rows, in the order they are tried.
func (t DecisionTable) Rows() []Row {
	out := make([]Row, len(t.rows))
	copy(out, t.rows)
	return out
}

// routes reports every treatment this table can send anyone to.
func (t DecisionTable) routes() []TreatmentID {
	var out []TreatmentID
	for _, row := range t.rows {
		if row.Then != "" {
			out = append(out, row.Then)
		}
	}
	if t.fallback.then != "" {
		out = append(out, t.fallback.then)
	}
	return out
}
