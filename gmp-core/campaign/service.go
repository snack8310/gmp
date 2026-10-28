package campaign

import (
	"errors"
	"fmt"
	"sort"

	"github.com/snack8310/gmp/gmp-core/audience"
)

var (
	// ErrDuplicateCampaign is returned when one campaign id is added twice.
	ErrDuplicateCampaign = errors.New("campaign: campaign added twice")
	// ErrUnbuiltCampaign is returned when a zero-value Campaign is added.
	ErrUnbuiltCampaign = errors.New("campaign: campaign was not built through a constructor")
	// ErrContentionUndecidable is returned when two campaigns want one slot,
	// both apply, and they rank equally.
	//
	// Nothing settles this. Priority is the whole rule, and the documents never
	// say what a tie means, so picking one would be inventing an answer and
	// then hiding it: whichever lost would look like it simply did not apply.
	// Failing loudly is the honest option until someone decides.
	ErrContentionUndecidable = errors.New("campaign: two campaigns rank equally for one slot and nothing decides between them")
)

// Service holds what has been registered and settles contests for a slot.
type Service struct {
	audiences *audience.Service
	slots     map[SlotID]Slot
	campaigns []Campaign
	byID      map[CampaignID]bool
}

// NewService builds a service over an audience service.
func NewService(audiences *audience.Service) *Service {
	return &Service{
		audiences: audiences,
		slots:     make(map[SlotID]Slot),
		byID:      make(map[CampaignID]bool),
	}
}

// RegisterSlot declares a slot this platform serves.
func (s *Service) RegisterSlot(slot Slot) error {
	if slot.isZero() {
		return ErrMissingSlotID
	}
	if _, taken := s.slots[slot.ID()]; taken {
		return fmt.Errorf("%w: %q", ErrDuplicateSlot, slot.ID())
	}
	s.slots[slot.ID()] = slot
	return nil
}

// Add puts a campaign into service. Every slot it wants must be registered.
func (s *Service) Add(c Campaign) error {
	if c.isZero() {
		return ErrUnbuiltCampaign
	}
	if s.audiences == nil {
		return errors.New("campaign: no audience service to validate groups")
	}
	if s.byID[c.id] {
		return fmt.Errorf("%w: %q", ErrDuplicateCampaign, c.id)
	}
	if c.hasRollout {
		if err := s.audiences.ValidateGroup(c.rollout); err != nil {
			return err
		}
	}
	for _, treatment := range c.treatments {
		if err := s.audiences.ValidateGroup(treatment.who); err != nil {
			return err
		}
	}
	for _, table := range c.tables {
		for _, row := range table.rows {
			if err := s.audiences.ValidateGroup(row.When); err != nil {
				return err
			}
		}
	}
	for _, slot := range c.slots() {
		if _, known := s.slots[slot]; !known {
			return fmt.Errorf("%w: %q, wanted by campaign %q", ErrUnknownSlot, slot, c.id)
		}
	}
	s.byID[c.id] = true
	s.campaigns = append(s.campaigns, c)
	return nil
}

// applicant is one campaign's answer for this slot before ranking.
type applicant struct {
	campaign  CampaignID
	priority  int
	treatment Treatment
	routeWhen audience.Group
	rollout   audience.Group
}

// Decide settles who gets the slot for this person, and produces what they
// should be shown.
//
// Nothing is executed and nothing is called out to: the result is a value. A
// rehearsal runs the identical path, which is the point -- a preview that took
// a different route would not be previewing anything.
func (s *Service) Decide(slot SlotID, uid audience.UID, mode audience.Mode) (Decision, error) {
	decision := Decision{Slot: slot, UID: uid}
	if slot == "" {
		return decision, ErrMissingSlotID
	}
	if _, known := s.slots[slot]; !known {
		return decision, fmt.Errorf("%w: %q", ErrUnknownSlot, slot)
	}
	if uid == "" {
		return decision, audience.ErrEmptyUID
	}
	if s.audiences == nil {
		return decision, errors.New("campaign: no audience service to ask who this is")
	}

	var applicants []applicant
	for _, c := range s.campaigns {
		candidate, routeWhen, losses, err := s.consider(c, slot, uid, mode)
		if err != nil {
			return decision, err
		}
		decision.Losses = append(decision.Losses, losses...)
		if candidate != nil {
			var rollout audience.Group
			if c.hasRollout {
				rollout = c.rollout
			}
			applicants = append(applicants, applicant{
				campaign:  c.id,
				priority:  c.priority.Value(),
				treatment: *candidate,
				routeWhen: routeWhen,
				rollout:   rollout,
			})
		}
	}
	if len(applicants) == 0 {
		return decision, nil
	}

	sort.SliceStable(applicants, func(i, j int) bool { return applicants[i].priority > applicants[j].priority })
	top := applicants[0]
	if len(applicants) > 1 && applicants[1].priority == top.priority {
		return decision, fmt.Errorf("%w: slot %q, campaigns %q and %q both rank %d",
			ErrContentionUndecidable, slot, top.campaign, applicants[1].campaign, top.priority)
	}

	placements, err := s.audiences.GroupPlacements(top.treatment.who, mode, uid)
	if err != nil {
		return decision, err
	}
	if top.routeWhen.ID() != "" {
		routed, err := s.audiences.GroupPlacements(top.routeWhen, mode, uid)
		if err != nil {
			return decision, err
		}
		placements = mergePlacements(placements, routed)
	}
	if top.rollout.ID() != "" {
		admitted, err := s.audiences.GroupPlacements(top.rollout, mode, uid)
		if err != nil {
			return decision, err
		}
		placements = mergePlacements(placements, admitted)
	}
	decision.Won = true
	decision.Award = Award{Campaign: top.campaign, Treatment: top.treatment.id, Call: top.treatment.call, Placements: placements}
	for _, loser := range applicants[1:] {
		decision.Losses = append(decision.Losses, Loss{
			Campaign:  loser.campaign,
			Treatment: loser.treatment.id,
			Reason:    LossOnPriority,
		})
	}
	return decision, nil
}

func mergePlacements(placements, routed []audience.Placement) []audience.Placement {
	for _, placement := range routed {
		found := false
		for _, existing := range placements {
			if existing.Assignment == placement.Assignment {
				found = true
				break
			}
		}
		if !found {
			placements = append(placements, placement)
		}
	}
	return placements
}

// consider works out this campaign's single answer for the slot, and records
// why anything else it holds did not get there.
func (s *Service) consider(c Campaign, slot SlotID, uid audience.UID, mode audience.Mode) (*Treatment, audience.Group, []Loss, error) {
	wants := false
	for _, want := range c.slots() {
		if want == slot {
			wants = true
			break
		}
	}
	if !wants {
		return nil, audience.Group{}, nil, nil
	}

	// The rollout gates the whole campaign at once. Applying it per treatment
	// would let arms of one experiment open up at different rates.
	if rollout, limited := c.Rollout(); limited {
		in, err := s.audiences.DecideGroup(rollout, mode, uid)
		if err != nil {
			return nil, audience.Group{}, nil, fmt.Errorf("campaign %q: checking the rollout: %w", c.id, err)
		}
		if !in {
			var losses []Loss
			for _, treatment := range c.treatments {
				if treatment.slot == slot {
					losses = append(losses, Loss{Campaign: c.id, Treatment: treatment.id, Reason: LossOutsideRollout})
				}
			}
			return nil, audience.Group{}, losses, nil
		}
	}

	if table, ordered := c.tables[slot]; ordered {
		return s.route(c, table, uid, mode)
	}

	var (
		chosen *Treatment
		losses []Loss
	)
	for i := range c.treatments {
		treatment := c.treatments[i]
		if treatment.slot != slot {
			continue
		}
		in, err := s.audiences.DecideGroup(treatment.who, mode, uid)
		if err != nil {
			return nil, audience.Group{}, nil, fmt.Errorf("campaign %q, treatment %q: %w", c.id, treatment.id, err)
		}
		if !in {
			losses = append(losses, Loss{Campaign: c.id, Treatment: treatment.id, Reason: LossAudienceDidNotMatch})
			continue
		}
		// Building the campaign established that at most one treatment on a
		// slot can apply, so a second match here would mean that check no
		// longer holds.
		if chosen != nil {
			return nil, audience.Group{}, nil, fmt.Errorf("%w: %q and %q both apply on slot %q in campaign %q",
				ErrSlotContestedWithinCampaign, chosen.id, treatment.id, slot, c.id)
		}
		chosen = &treatment
	}
	return chosen, audience.Group{}, losses, nil
}

// route walks a decision table top down and takes the first row that matches.
func (s *Service) route(c Campaign, table DecisionTable, uid audience.UID, mode audience.Mode) (*Treatment, audience.Group, []Loss, error) {
	held := make(map[TreatmentID]Treatment, len(c.treatments))
	for _, treatment := range c.treatments {
		held[treatment.id] = treatment
	}

	var losses []Loss
	for _, row := range table.rows {
		in, err := s.audiences.DecideGroup(row.When, mode, uid)
		if err != nil {
			return nil, audience.Group{}, nil, fmt.Errorf("campaign %q, table for slot %q: %w", c.id, table.slot, err)
		}
		if !in {
			if row.Then != "" {
				losses = append(losses, Loss{Campaign: c.id, Treatment: row.Then, Reason: LossRoutedElsewhere})
			}
			continue
		}
		if row.Then == "" {
			return nil, audience.Group{}, losses, nil
		}
		treatment := held[row.Then]
		if err := s.audiences.ValidateGroup(treatment.who); err != nil {
			return nil, audience.Group{}, nil, err
		}
		return &treatment, row.When, losses, nil
	}

	if table.fallback.then == "" {
		return nil, audience.Group{}, losses, nil
	}
	treatment := held[table.fallback.then]
	if err := s.audiences.ValidateGroup(treatment.who); err != nil {
		return nil, audience.Group{}, nil, err
	}
	return &treatment, audience.Group{}, losses, nil
}
