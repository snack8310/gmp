package scenarios

import (
	"fmt"

	"github.com/snack8310/gmp/gmp-core/audience"
	"github.com/snack8310/gmp/gmp-core/campaign"
)

// BannerContest is scenario 3 set up and ready to be asked: a promotion and a
// win-back campaign both want the home banner, and inside the promotion new and
// returning visitors see different images.
type BannerContest struct {
	Campaigns *campaign.Service
	Promotion campaign.CampaignID
	WinBack   campaign.CampaignID
}

// BannerContest builds scenario 3 on its own campaign service.
func (s *Stack) BannerContest() (*BannerContest, error) {
	newcomers, err := s.Definition("new-visitors", mustAttribute("tier", "new"))
	if err != nil {
		return nil, err
	}
	returning, err := s.Definition("returning-visitors", mustAttribute("tier", "returning"))
	if err != nil {
		return nil, err
	}
	everyone, err := s.Definition("anyone")
	if err != nil {
		return nil, err
	}

	promoA, err := treatment("promo-image-a", newcomers, "promo-a.png")
	if err != nil {
		return nil, err
	}
	promoB, err := treatment("promo-image-b", returning, "promo-b.png")
	if err != nil {
		return nil, err
	}
	table, err := campaign.NewDecisionTable(campaign.TableSpec{
		Slot: BannerSlot,
		Rows: []campaign.Row{
			{When: newcomers, Then: promoA.ID()},
			{When: returning, Then: promoB.ID()},
		},
		// Whoever is neither is left alone by this campaign, which is a
		// decision rather than an omission -- and it lets the lower-ranked
		// campaign have the slot instead of it going empty.
		Fallback: campaign.RouteNowhere(),
	})
	if err != nil {
		return nil, fmt.Errorf("scenarios: building the promotion's decision table: %w", err)
	}
	promotion, err := campaign.NewCampaign(campaign.CampaignSpec{
		ID:         "double-eleven",
		Priority:   campaign.NewPriority(100),
		Treatments: []campaign.Treatment{promoA, promoB},
		Tables:     []campaign.DecisionTable{table},
	})
	if err != nil {
		return nil, fmt.Errorf("scenarios: building the promotion: %w", err)
	}

	winbackBanner, err := treatment("winback-banner", everyone, "winback.png")
	if err != nil {
		return nil, err
	}
	winback, err := campaign.NewCampaign(campaign.CampaignSpec{
		ID:         "member-winback",
		Priority:   campaign.NewPriority(50),
		Treatments: []campaign.Treatment{winbackBanner},
	})
	if err != nil {
		return nil, fmt.Errorf("scenarios: building the win-back: %w", err)
	}

	service, err := s.NewCampaignService()
	if err != nil {
		return nil, err
	}
	for _, c := range []campaign.Campaign{promotion, winback} {
		if err := service.Add(c); err != nil {
			return nil, fmt.Errorf("scenarios: putting a campaign into service: %w", err)
		}
	}
	return &BannerContest{Campaigns: service, Promotion: promotion.ID(), WinBack: winback.ID()}, nil
}

// RolloutAdmitting is scenario 4 at one step of the knob: the campaign takes in
// everyone up to the given share of a hundred.
//
// Each step is its own service because a campaign is immutable; turning the
// knob means building another one, which is exactly what makes the steps
// comparable.
func (s *Stack) RolloutAdmitting(upTo int) (*campaign.Service, error) {
	assignment, err := EqualAssignment("gradual-rollout", 100, false)
	if err != nil {
		return nil, err
	}
	share, err := Share(upTo)
	if err != nil {
		return nil, fmt.Errorf("scenarios: rollout step %d: %w", upTo, err)
	}
	admitted, err := audience.ShareAtMost(assignment, share)
	if err != nil {
		return nil, fmt.Errorf("scenarios: building the rollout condition: %w", err)
	}
	inRollout, err := s.Definition("admitted-so-far", admitted)
	if err != nil {
		return nil, err
	}
	everyone, err := s.Definition("anyone")
	if err != nil {
		return nil, err
	}
	banner, err := treatment("new-mechanism-banner", everyone, "new-mechanism.png")
	if err != nil {
		return nil, err
	}
	gradual, err := campaign.NewCampaign(campaign.CampaignSpec{
		ID:         "gradual-launch",
		Priority:   campaign.NewPriority(10),
		Rollout:    inRollout,
		Treatments: []campaign.Treatment{banner},
	})
	if err != nil {
		return nil, fmt.Errorf("scenarios: building the gradual launch: %w", err)
	}
	service, err := s.NewCampaignService()
	if err != nil {
		return nil, err
	}
	if err := service.Add(gradual); err != nil {
		return nil, fmt.Errorf("scenarios: putting the gradual launch into service: %w", err)
	}
	return service, nil
}

// ExperimentArms is scenario 2's grouping: everyone who added to cart, carved
// into four arms, the first of which does nothing.
type ExperimentArms struct {
	Base audience.Definition
	// Order is the arm names in share order, so the control arm is first.
	Order []string
	Arms  map[string]audience.Definition
	// Control is the arm that hangs no treatments. It exists in its own right:
	// without it, its people could not be told apart afterwards from people who
	// were never in the audience at all.
	Control string
}

// ExperimentArms builds scenario 2's grouping.
func (s *Stack) ExperimentArms() (*ExperimentArms, error) {
	inCart := mustAttribute("added-to-cart", "yes")
	base, err := s.Definition("added-to-cart-30d", inCart)
	if err != nil {
		return nil, err
	}
	assignment, err := EqualAssignment(ArmAssignment, 4, true)
	if err != nil {
		return nil, err
	}
	order := []string{"A-control", "B-sms", "C-sms-and-coupon", "D-coupon"}
	arms := make(map[string]audience.Definition, len(order))
	for i, name := range order {
		share, err := Share(i + 1)
		if err != nil {
			return nil, err
		}
		inShare, err := audience.Share(assignment, share)
		if err != nil {
			return nil, fmt.Errorf("scenarios: building arm %q: %w", name, err)
		}
		definition, err := s.Definition(name, inCart, inShare)
		if err != nil {
			return nil, err
		}
		arms[name] = definition
	}
	return &ExperimentArms{Base: base, Order: order, Arms: arms, Control: order[0]}, nil
}

// mustAttribute builds an attribute condition whose inputs are fixed here, so
// it cannot fail for reasons a caller could act on.
func mustAttribute(name, value string) audience.Condition {
	condition, err := audience.Attribute(name, value)
	if err != nil {
		panic("scenarios: fixed attribute condition failed to build: " + err.Error())
	}
	return condition
}

func treatment(id string, who audience.Definition, image string) (campaign.Treatment, error) {
	action, err := campaign.RegisterAction(campaign.ActionSpec{
		ID: "show-image", Direction: campaign.DirectionPull, Required: []string{"image"},
	})
	if err != nil {
		return campaign.Treatment{}, fmt.Errorf("scenarios: registering the action: %w", err)
	}
	call, err := action.Call(map[string]string{"image": image})
	if err != nil {
		return campaign.Treatment{}, fmt.Errorf("scenarios: filling in the action: %w", err)
	}
	return campaign.NewTreatment(campaign.TreatmentSpec{
		ID:   campaign.TreatmentID(id),
		Slot: BannerSlot,
		Who:  who,
		Call: call,
	})
}

// ArmDelivery is scenario 2's delivery half for one arm: the people in that
// arm, and the machinery to push a message to each of them.
type ArmDelivery struct {
	Campaign  campaign.Campaign
	Treatment campaign.TreatmentID
	Runner    *campaign.Runner
	Log       *campaign.MemoryLog
	Deliverer *campaign.MemoryDeliverer
	// Arm is which share this treatment is aimed at.
	Arm int
}

// ArmDelivery builds the push side for one arm of scenario 2.
func (s *Stack) ArmDelivery(arm int) (*ArmDelivery, error) {
	arms, err := s.ExperimentArms()
	if err != nil {
		return nil, err
	}
	if arm < 1 || arm > len(arms.Order) {
		return nil, fmt.Errorf("scenarios: arm %d is outside the experiment", arm)
	}
	who := arms.Arms[arms.Order[arm-1]]

	action, err := campaign.RegisterAction(campaign.ActionSpec{
		ID: SMSAction, Direction: campaign.DirectionPush, Required: []string{"copy"},
	})
	if err != nil {
		return nil, fmt.Errorf("scenarios: registering the action: %w", err)
	}
	call, err := action.Call(map[string]string{"copy": "预热提醒 · 加购有礼"})
	if err != nil {
		return nil, fmt.Errorf("scenarios: filling in the action: %w", err)
	}
	treatment, err := campaign.NewTreatment(campaign.TreatmentSpec{
		ID: "warmup-sms", Slot: BannerSlot, Who: who, Call: call,
	})
	if err != nil {
		return nil, fmt.Errorf("scenarios: building the treatment: %w", err)
	}
	built, err := campaign.NewCampaign(campaign.CampaignSpec{
		ID:         "double-eleven-delivery",
		Priority:   campaign.NewPriority(100),
		Treatments: []campaign.Treatment{treatment},
	})
	if err != nil {
		return nil, fmt.Errorf("scenarios: building the campaign: %w", err)
	}

	log := campaign.NewMemoryLog()
	runner, err := campaign.NewRunner(campaign.RunnerSpec{
		Audiences: s.Audiences, Log: log, Events: campaign.DiscardEvents(), MaxAttempts: 3,
	})
	if err != nil {
		return nil, fmt.Errorf("scenarios: building the runner: %w", err)
	}
	deliverer := campaign.NewMemoryDeliverer()
	if err := runner.RegisterDeliverer(SMSAction, deliverer); err != nil {
		return nil, fmt.Errorf("scenarios: wiring the deliverer: %w", err)
	}
	return &ArmDelivery{
		Campaign: built, Treatment: treatment.ID(),
		Runner: runner, Log: log, Deliverer: deliverer, Arm: arm,
	}, nil
}

// ArmCampaign is scenario 2 in the shape operations would configure it: one
// campaign, with the arms of one assignment hanging under it, each arm doing
// its own thing.
//
// It is kept apart from ArmDelivery, which shows a different thing -- one
// arm's decision, delivery and receipt carrying a single key. Both draw their
// audiences from ExperimentArms, so the arms they talk about are the same
// arms.
type ArmCampaign struct {
	Campaign campaign.Campaign
	Arms     *ExperimentArms
	// Treatments is what hangs under the campaign, in build order.
	Treatments []ArmTreatment
	Runner     *campaign.Runner
	Log        *campaign.MemoryLog
	// Deliverers is what each action was wired to, so that a case can make a
	// send fail and check that a failure is still recorded in its arm.
	Deliverers map[campaign.ActionID]*campaign.MemoryDeliverer
}

// ArmTreatment is one treatment together with the arm it is aimed at.
type ArmTreatment struct {
	ID     campaign.TreatmentID
	Arm    string
	Slot   campaign.SlotID
	Action campaign.ActionID
}

// SMSAction and CouponAction are the two things scenario 2 does.
const (
	SMSAction    = campaign.ActionID("send-sms")
	CouponAction = campaign.ActionID("grant-coupon")
)

// ArmCampaign builds scenario 2 as a single campaign.
//
// The scenario lists a control arm, an arm that gets only a message, an arm
// that gets both a message and a coupon, and an arm that gets only a coupon.
// That is taken arm by arm: the control arm hangs nothing, and the arm that
// gets two things gets two treatments. Nothing new is introduced to express
// it -- the arms are ordinary audience definitions, and what separates two
// treatments on one slot is the proof that shares of one assignment never
// overlap.
func (s *Stack) ArmCampaign() (*ArmCampaign, error) {
	arms, err := s.ExperimentArms()
	if err != nil {
		return nil, err
	}

	sms, err := campaign.RegisterAction(campaign.ActionSpec{
		ID: SMSAction, Direction: campaign.DirectionPush, Required: []string{"copy"},
	})
	if err != nil {
		return nil, fmt.Errorf("scenarios: registering the message action: %w", err)
	}
	coupon, err := campaign.RegisterAction(campaign.ActionSpec{
		ID: CouponAction, Direction: campaign.DirectionPush, Required: []string{"coupon-id"},
	})
	if err != nil {
		return nil, fmt.Errorf("scenarios: registering the coupon action: %w", err)
	}
	smsCall, err := sms.Call(map[string]string{"copy": "预热提醒 · 加购有礼"})
	if err != nil {
		return nil, fmt.Errorf("scenarios: filling in the message action: %w", err)
	}
	couponCall, err := coupon.Call(map[string]string{"coupon-id": "double-eleven-20-off"})
	if err != nil {
		return nil, fmt.Errorf("scenarios: filling in the coupon action: %w", err)
	}

	// Arm by arm, in the order the scenario lists them. The control arm is
	// absent from this list on purpose, and its absence is what the regression
	// case checks: it is in the system as an audience, with nothing hung on it.
	planned := []struct {
		id   campaign.TreatmentID
		arm  string
		slot campaign.SlotID
		call campaign.Call
	}{
		{"warmup-sms-b", "B-sms", SMSSlot, smsCall},
		{"warmup-sms-c", "C-sms-and-coupon", SMSSlot, smsCall},
		{"warmup-coupon-c", "C-sms-and-coupon", CouponSlot, couponCall},
		{"warmup-coupon-d", "D-coupon", CouponSlot, couponCall},
	}

	treatments := make([]campaign.Treatment, 0, len(planned))
	described := make([]ArmTreatment, 0, len(planned))
	for _, p := range planned {
		who, held := arms.Arms[p.arm]
		if !held {
			return nil, fmt.Errorf("scenarios: treatment %q names arm %q, which is not in the experiment", p.id, p.arm)
		}
		built, err := campaign.NewTreatment(campaign.TreatmentSpec{
			ID: p.id, Slot: p.slot, Who: who, Call: p.call,
		})
		if err != nil {
			return nil, fmt.Errorf("scenarios: building treatment %q: %w", p.id, err)
		}
		treatments = append(treatments, built)
		described = append(described, ArmTreatment{
			ID: p.id, Arm: p.arm, Slot: p.slot, Action: p.call.Action(),
		})
	}

	// Two treatments share each slot here, and neither slot has a decision
	// table. What lets them through is the proof that different shares of one
	// assignment can never both hold; without it this call fails.
	built, err := campaign.NewCampaign(campaign.CampaignSpec{
		ID:         "double-eleven-experiment",
		Priority:   campaign.NewPriority(100),
		Treatments: treatments,
	})
	if err != nil {
		return nil, fmt.Errorf("scenarios: building the experiment campaign: %w", err)
	}

	log := campaign.NewMemoryLog()
	runner, err := campaign.NewRunner(campaign.RunnerSpec{
		Audiences: s.Audiences, Log: log, Events: campaign.DiscardEvents(), MaxAttempts: 3,
	})
	if err != nil {
		return nil, fmt.Errorf("scenarios: building the runner: %w", err)
	}
	deliverers := make(map[campaign.ActionID]*campaign.MemoryDeliverer, len(described))
	for _, action := range []campaign.ActionID{SMSAction, CouponAction} {
		deliverer := campaign.NewMemoryDeliverer()
		if err := runner.RegisterDeliverer(action, deliverer); err != nil {
			return nil, fmt.Errorf("scenarios: wiring the deliverer for %q: %w", action, err)
		}
		deliverers[action] = deliverer
	}

	return &ArmCampaign{
		Campaign: built, Arms: arms, Treatments: described,
		Runner: runner, Log: log, Deliverers: deliverers,
	}, nil
}

// ByArm reads off the campaign what each arm ends up with, keyed by arm name.
//
// This is a description of what was built, not a statement of what the
// scenario asks for. Checking the build against it would prove nothing --
// aim a treatment at the wrong arm and this moves with it. The regression
// case therefore states the scenario's own prescription itself.
func (a *ArmCampaign) ByArm() map[string][]campaign.ActionID {
	out := make(map[string][]campaign.ActionID, len(a.Arms.Order))
	for _, name := range a.Arms.Order {
		out[name] = nil
	}
	for _, treatment := range a.Treatments {
		out[treatment.Arm] = append(out[treatment.Arm], treatment.Action)
	}
	return out
}
