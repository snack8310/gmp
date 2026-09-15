package scenarios

import (
	"fmt"
	"time"

	"github.com/snack8310/gmp/gmp-core/audience"
	"github.com/snack8310/gmp/gmp-core/campaign"
)

// TwoStep is a scenario where one treatment's result is what lets the next one
// find anybody.
//
// The two treatments do not know about each other. The second is configured
// against an audience built over what flowed back, and would work just as well
// if something else entirely had produced those events.
type TwoStep struct {
	Backflow  *Backflow
	Runner    *campaign.Runner
	Log       *campaign.MemoryLog
	First     campaign.Campaign
	FirstStep campaign.TreatmentID
	Second    campaign.Campaign
	// SecondStep is empty when this scenario's second step cannot be expressed
	// in full; Missing then says what is missing.
	SecondStep campaign.TreatmentID
	Deliveries map[campaign.ActionID]*campaign.MemoryDeliverer
	// Missing records, in the scenario's own terms, what this build could not
	// express. Empty means nothing.
	Missing string
}

// WithBackflow registers this platform's own results as a second source, and
// returns the bridge so a scenario can publish into it and move its clock.
func (s *Stack) WithBackflow() (*Backflow, error) {
	backflow := NewBackflow()
	source, err := backflow.Source()
	if err != nil {
		return nil, err
	}
	if err := s.Audiences.Register(source, backflow); err != nil {
		return nil, fmt.Errorf("scenarios: registering the backflow source: %w", err)
	}
	return backflow, nil
}

// pushAction registers a push action with one parameter, the shape every step
// in these scenarios happens to need.
func pushAction(id campaign.ActionID, parameter string) (campaign.Action, campaign.Call, error) {
	action, err := campaign.RegisterAction(campaign.ActionSpec{
		ID: id, Direction: campaign.DirectionPush, Required: []string{parameter},
	})
	if err != nil {
		return campaign.Action{}, campaign.Call{}, fmt.Errorf("scenarios: registering %q: %w", id, err)
	}
	call, err := action.Call(map[string]string{parameter: "预设文案"})
	if err != nil {
		return campaign.Action{}, campaign.Call{}, fmt.Errorf("scenarios: filling in %q: %w", id, err)
	}
	return action, call, nil
}

// CrossBorderWinBack is scenario 5: reach the long-inactive on WhatsApp, and
// four hours later send a message to whoever has not read it.
//
// Every condition of the second step is something this platform's own results
// can answer: that we sent it, how long ago, and what the channel said about
// it afterwards.
func (s *Stack) CrossBorderWinBack() (*TwoStep, error) {
	backflow, err := s.WithBackflow()
	if err != nil {
		return nil, err
	}

	inactive, err := s.Definition("inactive-90-days", mustAttribute("tier", "other"))
	if err != nil {
		return nil, err
	}
	_, whatsapp, err := pushAction("send-whatsapp", "copy")
	if err != nil {
		return nil, err
	}
	firstStep, err := campaign.NewTreatment(campaign.TreatmentSpec{
		ID: "whatsapp-first", Slot: BannerSlot, Who: inactive, Call: whatsapp,
	})
	if err != nil {
		return nil, fmt.Errorf("scenarios: building the first step: %w", err)
	}
	first, err := campaign.NewCampaign(campaign.CampaignSpec{
		ID: "cross-border-winback", Priority: campaign.NewPriority(60),
		Treatments: []campaign.Treatment{firstStep},
	})
	if err != nil {
		return nil, fmt.Errorf("scenarios: building the first campaign: %w", err)
	}

	// The second step's audience: we sent it, at least four hours ago, and the
	// channel came back saying it was not read. Nothing here names the first
	// treatment.
	sent, err := audience.ElapsedSince(mustBackflowSource(), string(campaign.ArrivedEvent("send-whatsapp")), 4*time.Hour)
	if err != nil {
		return nil, fmt.Errorf("scenarios: building the elapsed condition: %w", err)
	}
	unread := mustAttribute("send-whatsapp.read", "no")
	unreadForHours, err := audience.NewDefinition(audience.DefinitionSpec{
		ID:         "whatsapp-unread-4h",
		Source:     BackflowSourceID,
		Conditions: []audience.Condition{sent, unread},
	})
	if err != nil {
		return nil, fmt.Errorf("scenarios: building the second step's audience: %w", err)
	}
	_, sms, err := pushAction("send-sms", "copy")
	if err != nil {
		return nil, err
	}
	secondStep, err := campaign.NewTreatment(campaign.TreatmentSpec{
		ID: "sms-follow-up", Slot: BannerSlot, Who: unreadForHours, Call: sms,
	})
	if err != nil {
		return nil, fmt.Errorf("scenarios: building the second step: %w", err)
	}
	second, err := campaign.NewCampaign(campaign.CampaignSpec{
		ID: "cross-border-followup", Priority: campaign.NewPriority(60),
		Treatments: []campaign.Treatment{secondStep},
	})
	if err != nil {
		return nil, fmt.Errorf("scenarios: building the second campaign: %w", err)
	}

	runner, log, deliveries, err := s.runnerOver(backflow, "send-whatsapp", "send-sms")
	if err != nil {
		return nil, err
	}
	return &TwoStep{
		Backflow: backflow, Runner: runner, Log: log,
		First: first, FirstStep: firstStep.ID(),
		Second: second, SecondStep: secondStep.ID(),
		Deliveries: deliveries,
	}, nil
}

// FirstOrderConversion is scenario 1: give a coupon to whoever has not ordered,
// then chase them three days later.
//
// The chase is only partly expressible. "Has not used it" is a fact the coupon
// system holds, and an audience definition draws on one source; whether one can
// span several is an open question in the audience documents, so it is left
// out rather than approximated.
func (s *Stack) FirstOrderConversion() (*TwoStep, error) {
	backflow, err := s.WithBackflow()
	if err != nil {
		return nil, err
	}

	noOrderYet, err := s.Definition("registered-no-order", mustAttribute("tier", "new"))
	if err != nil {
		return nil, err
	}
	_, coupon, err := pushAction("grant-coupon", "coupon-id")
	if err != nil {
		return nil, err
	}
	firstStep, err := campaign.NewTreatment(campaign.TreatmentSpec{
		ID: "coupon-first", Slot: BannerSlot, Who: noOrderYet, Call: coupon,
	})
	if err != nil {
		return nil, fmt.Errorf("scenarios: building the first step: %w", err)
	}
	first, err := campaign.NewCampaign(campaign.CampaignSpec{
		ID: "first-order-conversion", Priority: campaign.NewPriority(70),
		Treatments: []campaign.Treatment{firstStep},
	})
	if err != nil {
		return nil, fmt.Errorf("scenarios: building the first campaign: %w", err)
	}

	granted, err := audience.ElapsedSince(mustBackflowSource(), string(campaign.DeliveredEvent("grant-coupon")), 72*time.Hour)
	if err != nil {
		return nil, fmt.Errorf("scenarios: building the elapsed condition: %w", err)
	}
	threeDaysOn, err := audience.NewDefinition(audience.DefinitionSpec{
		ID:         "coupon-granted-3d",
		Source:     BackflowSourceID,
		Conditions: []audience.Condition{granted},
	})
	if err != nil {
		return nil, fmt.Errorf("scenarios: building the second step's audience: %w", err)
	}
	_, sms, err := pushAction("send-sms", "copy")
	if err != nil {
		return nil, err
	}
	secondStep, err := campaign.NewTreatment(campaign.TreatmentSpec{
		ID: "coupon-reminder", Slot: BannerSlot, Who: threeDaysOn, Call: sms,
	})
	if err != nil {
		return nil, fmt.Errorf("scenarios: building the second step: %w", err)
	}
	second, err := campaign.NewCampaign(campaign.CampaignSpec{
		ID: "first-order-reminder", Priority: campaign.NewPriority(70),
		Treatments: []campaign.Treatment{secondStep},
	})
	if err != nil {
		return nil, fmt.Errorf("scenarios: building the second campaign: %w", err)
	}

	runner, log, deliveries, err := s.runnerOver(backflow, "grant-coupon", "send-sms")
	if err != nil {
		return nil, err
	}
	return &TwoStep{
		Backflow: backflow, Runner: runner, Log: log,
		First: first, FirstStep: firstStep.ID(),
		Second: second, SecondStep: secondStep.ID(),
		Deliveries: deliveries,
		Missing:    "「未使用」来自券系统，属另一个来源；一份人群定义能否跨多个来源仍是受众侧的开放问题，故未表达",
	}, nil
}

// runnerOver builds a runner whose events flow into the backflow, with a
// deliverer wired up for each action the scenario uses.
//
// One runner serves both steps: the second step's deliveries are ordinary
// deliveries, and nothing about being second is special.
func (s *Stack) runnerOver(backflow *Backflow, actions ...campaign.ActionID) (*campaign.Runner, *campaign.MemoryLog, map[campaign.ActionID]*campaign.MemoryDeliverer, error) {
	log := campaign.NewMemoryLog()
	runner, err := campaign.NewRunner(campaign.RunnerSpec{
		Audiences: s.Audiences, Log: log, Events: backflow, MaxAttempts: 3,
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("scenarios: building the runner: %w", err)
	}
	deliveries := make(map[campaign.ActionID]*campaign.MemoryDeliverer, len(actions))
	for _, action := range actions {
		deliverer := campaign.NewMemoryDeliverer()
		if err := runner.RegisterDeliverer(action, deliverer); err != nil {
			return nil, nil, nil, fmt.Errorf("scenarios: wiring %q: %w", action, err)
		}
		deliveries[action] = deliverer
	}
	return runner, log, deliveries, nil
}

func mustBackflowSource() audience.Source {
	source, err := NewBackflow().Source()
	if err != nil {
		panic("scenarios: the backflow source failed to describe itself: " + err.Error())
	}
	return source
}
