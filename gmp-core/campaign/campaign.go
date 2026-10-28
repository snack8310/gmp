package campaign

import (
	"errors"
	"fmt"

	"github.com/snack8310/gmp/gmp-core/audience"
)

var (
	// ErrMissingCampaignID is returned when a campaign has no identity.
	ErrMissingCampaignID = errors.New("campaign: campaign id is empty")
	// ErrDuplicateTreatment is returned when one treatment id appears twice in
	// a campaign.
	ErrDuplicateTreatment = errors.New("campaign: treatment appears twice in one campaign")
	// ErrDuplicateTable is returned when a campaign has two tables for one slot.
	ErrDuplicateTable = errors.New("campaign: campaign has more than one decision table for a slot")
	// ErrRowTreatmentWrongSlot is returned when a table routes outside its display slot.
	ErrRowTreatmentWrongSlot = errors.New("campaign: decision table routes to a treatment outside its display slot")
	// ErrSlotContestedWithinCampaign is returned when a campaign puts two
	// treatments on one slot without either ordering them or being able to show
	// they can never both apply.
	//
	// Caught while building, not while serving. Left to run time it would mean
	// two of one campaign's treatments reaching the same person, which nothing
	// downstream would flag.
	ErrSlotContestedWithinCampaign = errors.New("campaign: two treatments contend for one slot with nothing to separate them")
)

// CampaignID identifies one campaign.
type CampaignID string

// Campaign is one piece of operational business, with treatments under it.
//
// Two things sit here rather than on the treatments: the priority used whenever
// its treatments contend for a slot, and the rollout. Both would break if they
// moved down -- separate rollouts per treatment would let experiment arms open
// up at different rates, and the conclusion would be wrong without anything
// failing.
//
// Values are immutable; everything is checked when the whole campaign is built,
// because the checks are about how its parts fit together.
type Campaign struct {
	id         CampaignID
	priority   Priority
	rollout    audience.Group
	hasRollout bool
	treatments []Treatment
	tables     map[SlotID]DecisionTable
	flow       Flow
}

// CampaignSpec is the input to building a campaign.
type CampaignSpec struct {
	// ID identifies the campaign.
	ID CampaignID
	// Priority ranks it against other campaigns at every slot.
	Priority Priority
	// Rollout, when set, is the group currently taken in, with a configuration
	// over the shares admitted so far. Leave it unset to admit everyone.
	Rollout audience.Group
	// Treatments are what this campaign does.
	Treatments []Treatment
	// Tables order the treatments that share a slot.
	Tables []DecisionTable
	// Flow, when set, is this campaign's flow: the edges between its
	// treatments. Leave it unset when every treatment is an entry.
	Flow Flow
}

// NewCampaign builds a campaign, refusing the arrangements that fail silently.
func NewCampaign(spec CampaignSpec) (Campaign, error) {
	if spec.ID == "" {
		return Campaign{}, ErrMissingCampaignID
	}
	if err := spec.Priority.validate(); err != nil {
		return Campaign{}, fmt.Errorf("%w: campaign %q", err, spec.ID)
	}

	treatments, byID, err := collectTreatments(spec.ID, spec.Treatments)
	if err != nil {
		return Campaign{}, err
	}
	tables, err := collectTables(spec.ID, spec.Tables, byID)
	if err != nil {
		return Campaign{}, err
	}
	if err := checkDisplayContention(spec.ID, treatments, tables); err != nil {
		return Campaign{}, err
	}

	var flow Flow
	if spec.Flow.id != "" {
		if err := validateFlow(spec.ID, spec.Flow, byID); err != nil {
			return Campaign{}, err
		}
		flow = spec.Flow
		flow.edges = append([]Edge(nil), spec.Flow.edges...)
	}

	return Campaign{
		id:         spec.ID,
		priority:   spec.Priority,
		rollout:    spec.Rollout,
		hasRollout: spec.Rollout.ID() != "",
		treatments: treatments,
		tables:     tables,
		flow:       flow,
	}, nil
}

func collectTreatments(id CampaignID, input []Treatment) ([]Treatment, map[TreatmentID]Treatment, error) {
	treatments := make([]Treatment, 0, len(input))
	byID := make(map[TreatmentID]Treatment, len(input))
	for _, treatment := range input {
		if treatment.id == "" {
			return nil, nil, fmt.Errorf("%w: campaign %q", ErrMissingTreatmentID, id)
		}
		if _, taken := byID[treatment.id]; taken {
			return nil, nil, fmt.Errorf("%w: %q in campaign %q", ErrDuplicateTreatment, treatment.id, id)
		}
		byID[treatment.id] = treatment
		treatments = append(treatments, treatment)
	}
	return treatments, byID, nil
}

func collectTables(id CampaignID, input []DecisionTable, byID map[TreatmentID]Treatment) (map[SlotID]DecisionTable, error) {
	tables := make(map[SlotID]DecisionTable, len(input))
	for _, table := range input {
		if _, taken := tables[table.slot]; taken {
			return nil, fmt.Errorf("%w: %q in campaign %q", ErrDuplicateTable, table.slot, id)
		}
		for _, routed := range table.routes() {
			treatment, held := byID[routed]
			if !held {
				return nil, fmt.Errorf("%w: %q, slot %q, campaign %q",
					ErrRowTreatmentNotInCampaign, routed, table.slot, id)
			}
			if treatment.slot != table.slot {
				return nil, fmt.Errorf("%w: %q, slot %q, campaign %q", ErrRowTreatmentWrongSlot, routed, table.slot, id)
			}
		}
		tables[table.slot] = table
	}
	return tables, nil
}

// Two display treatments on one slot need a table or provably exclusive audiences.
func checkDisplayContention(id CampaignID, treatments []Treatment, tables map[SlotID]DecisionTable) error {
	bySlot := map[SlotID][]Treatment{}
	for _, treatment := range treatments {
		if treatment.slot != "" {
			bySlot[treatment.slot] = append(bySlot[treatment.slot], treatment)
		}
	}
	for slot, sharing := range bySlot {
		if len(sharing) < 2 {
			continue
		}
		if _, ordered := tables[slot]; ordered {
			continue
		}
		for i := range sharing {
			for j := i + 1; j < len(sharing); j++ {
				if !audience.ProvablyExclusive(sharing[i].who.Definition(), sharing[j].who.Definition()) {
					return fmt.Errorf("%w: %q and %q on slot %q in campaign %q; order them with a decision table, or give them shares of one assignment",
						ErrSlotContestedWithinCampaign, sharing[i].id, sharing[j].id, slot, id)
				}
			}
		}
	}
	return nil
}

// ID reports which campaign this is.
func (c Campaign) ID() CampaignID { return c.id }

// Priority reports how it ranks against other campaigns.
func (c Campaign) Priority() Priority { return c.priority }

// Rollout reports who is currently taken in, and whether the campaign limits
// that at all.
func (c Campaign) Rollout() (audience.Group, bool) { return c.rollout, c.hasRollout }

// Treatments reports what this campaign does.
func (c Campaign) Treatments() []Treatment {
	out := make([]Treatment, len(c.treatments))
	copy(out, c.treatments)
	return out
}

func (c Campaign) isZero() bool { return c.id == "" }

// slots reports every slot this campaign wants.
func (c Campaign) slots() []SlotID {
	seen := map[SlotID]bool{}
	var out []SlotID
	for _, treatment := range c.treatments {
		if treatment.slot == "" {
			continue
		}
		if !seen[treatment.slot] {
			seen[treatment.slot] = true
			out = append(out, treatment.slot)
		}
	}
	for slot := range c.tables {
		if !seen[slot] {
			seen[slot] = true
			out = append(out, slot)
		}
	}
	return out
}
