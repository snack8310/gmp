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
	assignment, err := EqualAssignment("double-eleven-warmup", 4, true)
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
	action, err := campaign.RegisterAction("show-image", "image")
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
