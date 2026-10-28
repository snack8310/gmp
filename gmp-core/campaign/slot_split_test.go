package campaign_test

import (
	"errors"
	"testing"

	"github.com/snack8310/gmp/gmp-core/campaign"
)

func TestPushTreatmentsHaveNoDisplaySlot(t *testing.T) {
	w := newWorld(t, 3)
	who := w.definition("everyone")
	action, err := campaign.RegisterAction(campaign.ActionSpec{ID: "send-message", Direction: campaign.DirectionPush})
	if err != nil {
		t.Fatal(err)
	}
	call, err := action.Call(nil)
	if err != nil {
		t.Fatal(err)
	}
	first, err := campaign.NewTreatment(campaign.TreatmentSpec{ID: "first", Who: who, Call: call})
	if err != nil {
		t.Fatalf("push without a slot: %v", err)
	}
	second, err := campaign.NewTreatment(campaign.TreatmentSpec{ID: "second", Who: who, Call: call})
	if err != nil {
		t.Fatalf("second push without a slot: %v", err)
	}
	if first.Slot() != "" || second.Slot() != "" {
		t.Fatalf("push treatments occupy display slots: %q, %q", first.Slot(), second.Slot())
	}
	built, err := campaign.NewCampaign(campaign.CampaignSpec{
		ID: "push-only", Priority: campaign.NewPriority(1), Treatments: []campaign.Treatment{first, second},
	})
	if err != nil {
		t.Fatalf("two push treatments in one campaign: %v", err)
	}
	if err := campaign.NewService(w.audiences).Add(built); err != nil {
		t.Fatalf("push-only campaign needs no registered slot: %v", err)
	}
	for _, slot := range []campaign.SlotID{bannerID, "sms", "coupon"} {
		if _, err := campaign.NewTreatment(campaign.TreatmentSpec{ID: "invalid-push", Slot: slot, Who: who, Call: call}); !errors.Is(err, campaign.ErrUnexpectedSlotID) {
			t.Fatalf("push with display slot %q returned %v, want ErrUnexpectedSlotID", slot, err)
		}
	}
}

func TestMarkedDisplayActionRequiresSlot(t *testing.T) {
	w := newWorld(t, 3)
	who := w.definition("everyone")
	call := w.call("image.png")
	if _, err := campaign.NewTreatment(campaign.TreatmentSpec{ID: "missing", Who: who, Call: call}); !errors.Is(err, campaign.ErrMissingSlotID) {
		t.Fatalf("display action without a slot returned %v, want ErrMissingSlotID", err)
	}
	if _, err := campaign.NewTreatment(campaign.TreatmentSpec{ID: "banner", Slot: bannerID, Who: who, Call: call}); err != nil {
		t.Fatalf("display action with banner slot: %v", err)
	}
}

func TestUnmarkedPullActionDoesNotRequireDisplaySlot(t *testing.T) {
	w := newWorld(t, 3)
	action, err := campaign.RegisterAction(campaign.ActionSpec{ID: "lookup", Direction: campaign.DirectionPull})
	if err != nil {
		t.Fatal(err)
	}
	call, err := action.Call(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := campaign.NewTreatment(campaign.TreatmentSpec{ID: "lookup", Who: w.definition("everyone"), Call: call}); err != nil {
		t.Fatalf("unmarked pull action was forced into a display slot: %v", err)
	}
	if _, err := campaign.NewTreatment(campaign.TreatmentSpec{ID: "lookup-on-banner", Slot: bannerID, Who: w.definition("everyone"), Call: call}); !errors.Is(err, campaign.ErrUnexpectedSlotID) {
		t.Fatalf("unmarked pull action with a display slot returned %v, want ErrUnexpectedSlotID", err)
	}
}

func TestPushActionCannotClaimItRequiresDisplaySlot(t *testing.T) {
	if _, err := campaign.RegisterAction(campaign.ActionSpec{
		ID: "invalid-push", Direction: campaign.DirectionPush, RequiresSlot: true,
	}); err == nil {
		t.Fatal("push action claimed a display slot")
	}
}

func TestDecisionTableCannotRoutePushIntoDisplaySlot(t *testing.T) {
	w := newWorld(t, 3)
	who := w.definition("everyone")
	action, err := campaign.RegisterAction(campaign.ActionSpec{ID: "send-message", Direction: campaign.DirectionPush})
	if err != nil {
		t.Fatal(err)
	}
	call, err := action.Call(nil)
	if err != nil {
		t.Fatal(err)
	}
	push, err := campaign.NewTreatment(campaign.TreatmentSpec{ID: "push", Who: who, Call: call})
	if err != nil {
		t.Fatal(err)
	}
	for _, spec := range []struct {
		name     string
		rows     []campaign.Row
		fallback campaign.Fallback
	}{
		{"row", []campaign.Row{{When: who, Then: push.ID()}}, campaign.RouteNowhere()},
		{"fallback", nil, campaign.RouteTo(push.ID())},
	} {
		t.Run(spec.name, func(t *testing.T) {
			table, err := campaign.NewDecisionTable(campaign.TableSpec{Slot: bannerID, Rows: spec.rows, Fallback: spec.fallback})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := campaign.NewCampaign(campaign.CampaignSpec{
				ID: "push-on-banner", Priority: campaign.NewPriority(1),
				Treatments: []campaign.Treatment{push}, Tables: []campaign.DecisionTable{table},
			}); !errors.Is(err, campaign.ErrRowTreatmentWrongSlot) {
				t.Fatalf("decision table routed a push treatment into a display slot: %v", err)
			}
		})
	}
}
