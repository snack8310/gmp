package campaign

import "github.com/snack8310/gmp/gmp-core/audience"

// LossReason says why a treatment did not get the slot.
type LossReason string

const (
	// LossOutsideRollout means this person is not among the shares taken in so
	// far, so nothing under the campaign applies to them yet.
	LossOutsideRollout LossReason = "outside the rollout"
	// LossAudienceDidNotMatch means the treatment's audience does not hold for
	// this person.
	LossAudienceDidNotMatch LossReason = "audience did not match"
	// LossRoutedElsewhere means a decision table sent this person to another
	// row, or to nowhere.
	LossRoutedElsewhere LossReason = "routed elsewhere by the decision table"
	// LossOnPriority means the treatment applied but another campaign outranked
	// it.
	LossOnPriority LossReason = "lost on priority"
)

// Loss records one campaign not getting the slot, and why.
//
// Every loss is recorded, not only the closest one. Without this an operator
// asking why their campaign reaches so few people has nowhere to look; a
// campaign that loses every contest looks identical to one nobody configured.
//
// Losing here is not being bothered: nothing was sent. It is also not being out
// of the experiment -- whoever was assigned to an arm stays in it, the same way
// someone the send failed for does.
type Loss struct {
	Campaign  CampaignID
	Treatment TreatmentID
	Reason    LossReason
}

// Award is the treatment that got the slot, and what it produced.
type Award struct {
	Campaign  CampaignID
	Treatment TreatmentID
	Call      Call
}

// Decision is the whole outcome of one contest for one slot: what to show, and
// what did not get shown.
//
// It is a value. Nothing here calls out to anything, which is what makes it
// possible to work out what someone would receive without sending it.
type Decision struct {
	Slot   SlotID
	UID    audience.UID
	Won    bool
	Award  Award
	Losses []Loss
}
