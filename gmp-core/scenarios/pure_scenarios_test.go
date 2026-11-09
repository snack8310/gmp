package scenarios_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/snack8310/gmp/gmp-core/audience"
	"github.com/snack8310/gmp/gmp-core/campaign"
	"github.com/snack8310/gmp/gmp-core/experimentation"
	"github.com/snack8310/gmp/gmp-core/scenarios"
)

func TestScenarioCatalogListsFiveEstablishedStories(t *testing.T) {
	ids := []scenarios.ID{
		scenarios.NewCustomerFirstOrder,
		scenarios.DoubleElevenWarmup,
		scenarios.HomeBannerPersonalize,
		scenarios.NewPlayRamp,
		scenarios.CrossBorderRecall,
	}
	seen := make(map[scenarios.ID]bool, len(ids))
	for _, id := range ids {
		if id == "" || seen[id] {
			t.Fatalf("scenario catalog contains an empty or duplicate id: %q", id)
		}
		seen[id] = true
	}
}

func TestNewCustomerScenarioWaitsThreeDaysFromCouponDelivery(t *testing.T) {
	registrationSource := scenarioSource(t, "new-customer-registration", audience.CategoryExternal, time.Hour)
	registeredFor24Hours, err := audience.ElapsedSince(registrationSource, "registered", 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	entry := scenarioGroup(t, "new-customer-entry", registrationSource.ID(), registeredFor24Hours)
	entryRecord := audience.Record{
		UID: "uid_new_customer", EventAges: map[string]time.Duration{"registered": 24 * time.Hour},
	}
	eligible, err := entry.Matches(entryRecord, nil)
	if err != nil || !eligible {
		t.Fatalf("24-hour registration match = %v, err=%v", eligible, err)
	}

	backflow := scenarioSource(t, "new-customer-coupon-backflow", audience.CategoryBackflow, time.Hour)
	coupon := scenarioPushAction(t, "new-customer-coupon", campaign.OutcomeSuccess, campaign.OutcomeFailure)
	sms := scenarioPushAction(t, "new-customer-sms", campaign.OutcomeSuccess, campaign.OutcomeFailure)
	deliveryEventName := campaign.DeliveredEvent(coupon.ID())
	threeDays, err := audience.ElapsedSince(backflow, string(deliveryEventName), 72*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	reminder := scenarioGroup(t, "new-customer-reminder-window", backflow.ID(), threeDays)
	couponTreatment := scenarioTreatment(t, "new-customer-coupon-step", coupon, nil)
	smsTreatment := scenarioTreatment(t, "new-customer-sms-step", sms, nil)
	entryFlow, err := campaign.NewFlow(campaign.FlowSpec{
		ID:      "new-customer-entry-flow",
		Entries: []campaign.EntrySpec{{Group: entry.ID(), Kind: campaign.EntryStart, Treatments: []campaign.TreatmentID{couponTreatment.ID()}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	entryCampaign := scenarioCampaign(t, "new-customer-campaign", 10, audience.SelectGroup(entry), []campaign.Treatment{couponTreatment}, entryFlow, nil)
	tooEarlyEntry := audience.Record{
		UID: "uid_new_customer", EventAges: map[string]time.Duration{"registered": 23*time.Hour + 59*time.Minute},
	}
	if _, shouldDeliver, err := campaign.PlanEntry(entryCampaign, couponTreatment.ID(), tooEarlyEntry, nil, "new-customer-before-registration-window"); err != nil || shouldDeliver {
		t.Fatalf("pre-24-hour coupon plan = deliver %v, err=%v; want no coupon", shouldDeliver, err)
	}
	reminderFlow, err := campaign.NewFlow(campaign.FlowSpec{
		ID:      "new-customer-reminder-flow",
		Entries: []campaign.EntrySpec{{Group: reminder.ID(), Kind: campaign.EntryStart, Treatments: []campaign.TreatmentID{smsTreatment.ID()}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	reminderCampaign := scenarioCampaign(t, "new-customer-reminder-campaign", 10, audience.SelectGroup(reminder), []campaign.Treatment{smsTreatment}, reminderFlow, nil)
	couponIntent, shouldDeliver, err := campaign.PlanEntry(entryCampaign, couponTreatment.ID(), entryRecord, nil, "new-customer-registration-1")
	if err != nil || !shouldDeliver || couponIntent.UID != entryRecord.UID {
		t.Fatalf("24-hour registration coupon plan = %#v, deliver=%v, err=%v", couponIntent, shouldDeliver, err)
	}
	execution, err := campaign.CompleteDelivery(couponIntent, 1, campaign.Ack{ExternalRef: "constructed-coupon-ref"}, true)
	if err != nil {
		t.Fatal(err)
	}
	couponEvent := campaign.Event{
		Name: deliveryEventName, UID: execution.UID, Key: execution.Key,
		Campaign: execution.Campaign, Treatment: execution.Treatment, Action: execution.Action,
	}
	if couponEvent.Name != campaign.EventName("new-customer-coupon:delivered") || couponEvent.Key != execution.Key || couponEvent.UID != execution.UID {
		t.Fatalf("coupon delivery event = %#v, want the existing delivered event linked to the coupon execution", couponEvent)
	}
	backflowRecord := audience.Record{
		UID:       couponEvent.UID,
		EventAges: map[string]time.Duration{string(couponEvent.Name): 72 * time.Hour},
	}
	tooSoon := audience.Record{
		UID:       backflowRecord.UID,
		EventAges: map[string]time.Duration{string(couponEvent.Name): 71*time.Hour + 59*time.Minute},
	}
	if _, shouldDeliver, err := campaign.PlanEntry(reminderCampaign, smsTreatment.ID(), tooSoon, nil, "new-customer-reminder-before-window"); err != nil || shouldDeliver {
		t.Fatalf("pre-window reminder plan = deliver %v, err=%v; want no SMS", shouldDeliver, err)
	}
	reminderIntent, shouldDeliver, err := campaign.PlanEntry(reminderCampaign, smsTreatment.ID(), backflowRecord, nil, "new-customer-reminder-1")
	if err != nil || !shouldDeliver || reminderIntent.UID != execution.UID {
		t.Fatalf("three-day reminder plan = %#v, deliver=%v, err=%v", reminderIntent, shouldDeliver, err)
	}
}

func TestDoubleElevenScenarioPartitionsFourArmsWithExplicitControl(t *testing.T) {
	source := scenarioSource(t, "double-eleven-source", audience.CategoryExternal, 0)
	assignment := scenarioAssignment(t, "double-eleven-assignment", 4, false)
	shares := make([]experimentation.BucketNumber, 4)
	arms := make([]audience.BundleArm, 4)
	groups := make(map[int]audience.GroupID, 4)
	for index := range shares {
		shares[index] = scenarioBucket(t, index+1)
		condition, err := audience.Share(assignment, shares[index])
		if err != nil {
			t.Fatal(err)
		}
		definition := scenarioDefinition(t, fmt.Sprintf("double-eleven-arm-%d", index+1), source.ID(), condition)
		groupID := audience.GroupID(fmt.Sprintf("double-eleven-group-%d", index+1))
		groups[index+1] = groupID
		arms[index] = audience.BundleArm{ID: groupID, Share: shares[index], Definition: definition}
	}
	bundle, err := audience.NewBundle(audience.BundleSpec{
		ID: "double-eleven-bundle", Source: source.ID(), Assignment: assignment, Arms: arms,
	})
	if err != nil {
		t.Fatal(err)
	}
	component, err := experimentation.NewComponentState(assignment)
	if err != nil {
		t.Fatal(err)
	}
	active, err := component.Start()
	if err != nil {
		t.Fatal(err)
	}
	bundle, err = bundle.Publish(active)
	if err != nil {
		t.Fatal(err)
	}

	flowEntries := make([]campaign.EntrySpec, 4)
	treatments := make([]campaign.Treatment, 0, 3)
	for share := 1; share <= 4; share++ {
		entry := campaign.EntrySpec{Group: groups[share]}
		if share == 1 {
			entry.Kind = campaign.EntryControl
		} else {
			entry.Kind = campaign.EntryStart
			action := scenarioPushAction(t, fmt.Sprintf("double-eleven-action-%d", share))
			treatment := scenarioTreatment(t, fmt.Sprintf("double-eleven-treatment-%d", share), action, nil)
			treatments = append(treatments, treatment)
			entry.Treatments = []campaign.TreatmentID{treatment.ID()}
		}
		flowEntries[share-1] = entry
	}
	flow, err := campaign.NewFlow(campaign.FlowSpec{ID: "double-eleven-flow", Entries: flowEntries})
	if err != nil {
		t.Fatal(err)
	}
	built, err := campaign.NewCampaign(campaign.CampaignSpec{
		ID: "double-eleven-campaign", Priority: campaign.NewPriority(10), Audience: audience.SelectBundle(bundle),
		Treatments: treatments, Flow: flow,
	})
	if err != nil {
		t.Fatal(err)
	}
	for share := 1; share <= 4; share++ {
		group, found := bundle.Group(shares[share-1])
		if !found {
			t.Fatalf("bundle is missing share %d", share)
		}
		selection := audience.SelectBundle(bundle)
		record := audience.Record{UID: audience.UID(fmt.Sprintf("uid-arm-%d", share))}
		facts := audience.AssignmentFacts{{Definition: assignment, Bucket: shares[share-1]}}
		matchedGroup, matched, err := selection.Match(
			record,
			facts,
		)
		if err != nil || !matched || matchedGroup.ID() != group.ID() {
			t.Fatalf("share %d match = %q, %v, err=%v", share, matchedGroup.ID(), matched, err)
		}
		route, found := flow.RouteForGroup(group.ID())
		if !found {
			t.Fatalf("share %d has no explicit flow route", share)
		}
		if share == 1 && route.Kind() != campaign.EntryControl {
			t.Fatalf("control share route = %q", route.Kind())
		}
		if share == 1 {
			if len(route.Treatments()) != 0 {
				t.Fatalf("control share %d has treatments: %#v", share, route.Treatments())
			}
			continue
		}
		if route.Kind() != campaign.EntryStart || len(route.Treatments()) != 1 {
			t.Fatalf("treatment share %d route = %#v", share, route)
		}
		intent, shouldDeliver, err := campaign.PlanEntry(built, route.Treatments()[0], record, facts, campaign.Occurrence(fmt.Sprintf("double-eleven-%d", share)))
		if err != nil || !shouldDeliver {
			t.Fatalf("share %d entry plan = %#v, deliver=%v, err=%v", share, intent, shouldDeliver, err)
		}
		if intent.UID != record.UID || intent.Treatment != route.Treatments()[0] || intent.Placements == nil || len(intent.Placements) != 1 ||
			intent.Placements[0].Assignment != string(assignment.ID()) || intent.Placements[0].Share != share {
			t.Fatalf("share %d delivery intent = %#v, want matching UID, treatment and placement", share, intent)
		}
		execution, err := campaign.CompleteDelivery(intent, 1, campaign.Ack{ExternalRef: "constructed-double-eleven-ref"}, true)
		if err != nil || execution.UID != record.UID || execution.Treatment != intent.Treatment || execution.State != campaign.StateDelivered {
			t.Fatalf("share %d execution = %#v, err=%v", share, execution, err)
		}
	}
}

func TestBannerScenarioUsesDecisionTableAndCampaignPriority(t *testing.T) {
	source := scenarioSource(t, "banner-source", audience.CategoryExternal, 0)
	all := scenarioGroup(t, "banner-all", source.ID())
	newAttribute, err := audience.Attribute("customer_type", "new")
	if err != nil {
		t.Fatal(err)
	}
	oldAttribute, err := audience.Attribute("customer_type", "existing")
	if err != nil {
		t.Fatal(err)
	}
	newGroup := scenarioGroup(t, "banner-new", source.ID(), newAttribute)
	oldGroup := scenarioGroup(t, "banner-existing", source.ID(), oldAttribute)
	slot, err := campaign.NewSlot("home-banner")
	if err != nil {
		t.Fatal(err)
	}
	newAction := scenarioDisplayAction(t, "banner-new-action")
	oldAction := scenarioDisplayAction(t, "banner-existing-action")
	recallAction := scenarioDisplayAction(t, "banner-recall-action")
	newTreatment := scenarioTreatment(t, "banner-new-treatment", newAction, &slot)
	oldTreatment := scenarioTreatment(t, "banner-existing-treatment", oldAction, &slot)
	recallTreatment := scenarioTreatment(t, "banner-recall-treatment", recallAction, &slot)
	table, err := campaign.NewDecisionTable(campaign.TableSpec{
		Slot: slot.ID(), Rows: []campaign.Row{{When: newGroup, Then: newTreatment.ID()}, {When: oldGroup, Then: oldTreatment.ID()}},
		Fallback: campaign.RouteNowhere(),
	})
	if err != nil {
		t.Fatal(err)
	}
	festivalFlow, err := campaign.NewFlow(campaign.FlowSpec{
		ID: "banner-festival-flow", Entries: []campaign.EntrySpec{{
			Group: all.ID(), Kind: campaign.EntryStart,
			Treatments: []campaign.TreatmentID{newTreatment.ID(), oldTreatment.ID()},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	festival := scenarioCampaign(t, "banner-festival", 100, audience.SelectGroup(all),
		[]campaign.Treatment{newTreatment, oldTreatment}, festivalFlow, []campaign.DecisionTable{table})
	recallFlow, err := campaign.NewFlow(campaign.FlowSpec{
		ID: "banner-recall-flow", Entries: []campaign.EntrySpec{{Group: all.ID(), Kind: campaign.EntryStart, Treatments: []campaign.TreatmentID{recallTreatment.ID()}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	recall := scenarioCampaign(t, "banner-recall", 50, audience.SelectGroup(all), []campaign.Treatment{recallTreatment}, recallFlow, nil)

	for _, test := range []struct {
		customerType string
		want         campaign.TreatmentID
	}{
		{customerType: "new", want: newTreatment.ID()},
		{customerType: "existing", want: oldTreatment.ID()},
	} {
		decision, err := campaign.Decide(
			[]campaign.Campaign{recall, festival}, []campaign.Slot{slot}, slot.ID(),
			audience.Record{UID: "uid_banner", Attributes: map[string]string{"customer_type": test.customerType}}, nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		if !decision.Won || decision.Award.Campaign != festival.ID() || decision.Award.Treatment != test.want {
			t.Fatalf("%s banner decision = %#v, want festival treatment %q", test.customerType, decision, test.want)
		}
		recallLostOnPriority := false
		for _, loss := range decision.Losses {
			if loss.Campaign == recall.ID() && loss.Reason == campaign.LossOnPriority {
				recallLostOnPriority = true
			}
		}
		if !recallLostOnPriority {
			t.Fatalf("recall loss = %#v, want a recorded priority loss", decision.Losses)
		}
	}
}

func TestRampScenarioOnlyExpandsTheAdmittedShareSet(t *testing.T) {
	assignment := scenarioAssignment(t, "ramp-assignment", 100, false)
	five, err := audience.ShareAtMost(assignment, scenarioBucket(t, 5))
	if err != nil {
		t.Fatal(err)
	}
	twenty, err := audience.ShareAtMost(assignment, scenarioBucket(t, 20))
	if err != nil {
		t.Fatal(err)
	}
	oneHundred, err := audience.ShareAtMost(assignment, scenarioBucket(t, 100))
	if err != nil {
		t.Fatal(err)
	}
	definitions := []audience.Definition{
		scenarioDefinition(t, "ramp-five-percent", "ramp-source", five),
		scenarioDefinition(t, "ramp-twenty-percent", "ramp-source", twenty),
		scenarioDefinition(t, "ramp-full", "ramp-source", oneHundred),
	}
	seenFive, seenTwenty := false, false
	for index := 0; index < 10_000; index++ {
		uid := audience.UID(fmt.Sprintf("uid_ramp_%d", index))
		bucket, err := experimentation.Calculate(assignment, uid.AssignmentKey(), experimentation.FNV1aV1{})
		if err != nil {
			t.Fatal(err)
		}
		facts := audience.AssignmentFacts{{Definition: assignment, Bucket: bucket}}
		record := audience.Record{UID: uid}
		admitted := make([]bool, 3)
		for position, definition := range definitions {
			admitted[position], err = definition.Matches(record, facts)
			if err != nil {
				t.Fatal(err)
			}
		}
		if admitted[0] && !admitted[1] {
			t.Fatalf("uid %q left the rollout after widening", uid)
		}
		if !admitted[2] {
			t.Fatalf("uid %q was excluded at full rollout", uid)
		}
		seenFive = seenFive || admitted[0]
		seenTwenty = seenTwenty || admitted[1]
	}
	if !seenFive || !seenTwenty {
		t.Fatalf("test sample did not exercise both rollout stages: five=%v twenty=%v", seenFive, seenTwenty)
	}
}

func TestCrossBorderScenarioUsesFourHourUnreadFactsAndReceiptIdentity(t *testing.T) {
	backflow := scenarioSource(t, "cross-border-backflow", audience.CategoryBackflow, time.Hour)
	whatsapp := scenarioPushAction(t, "cross-border-whatsapp", campaign.OutcomeSuccess, campaign.OutcomeFailure)
	sms := scenarioPushAction(t, "cross-border-sms", campaign.OutcomeSuccess, campaign.OutcomeFailure)
	receiptEvent := campaign.ArrivedEvent(whatsapp.ID())
	fourHours, err := audience.ElapsedSince(backflow, string(receiptEvent), 4*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	unread, err := audience.Attribute("read", "no")
	if err != nil {
		t.Fatal(err)
	}
	reminder := scenarioDefinition(t, "cross-border-reminder", backflow.ID(), fourHours, unread)

	entrySource := scenarioSource(t, "cross-border-entry-source", audience.CategoryExternal, 0)
	entry := scenarioGroup(t, "cross-border-entry", entrySource.ID())
	reminderGroup := audience.Single(reminder)
	first := scenarioTreatment(t, "cross-border-first", whatsapp, nil)
	next := scenarioTreatment(t, "cross-border-next", sms, nil)
	entryFlow, err := campaign.NewFlow(campaign.FlowSpec{
		ID:      "cross-border-flow",
		Entries: []campaign.EntrySpec{{Group: entry.ID(), Kind: campaign.EntryStart, Treatments: []campaign.TreatmentID{first.ID()}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	entryCampaign := scenarioCampaign(t, "cross-border-campaign", 10, audience.SelectGroup(entry), []campaign.Treatment{first}, entryFlow, nil)
	reminderFlow, err := campaign.NewFlow(campaign.FlowSpec{
		ID:      "cross-border-reminder-flow",
		Entries: []campaign.EntrySpec{{Group: reminderGroup.ID(), Kind: campaign.EntryStart, Treatments: []campaign.TreatmentID{next.ID()}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	reminderCampaign := scenarioCampaign(t, "cross-border-reminder-campaign", 10, audience.SelectGroup(reminderGroup), []campaign.Treatment{next}, reminderFlow, nil)
	entryRecord := audience.Record{UID: "uid_cross_border"}
	intent, shouldDeliver, err := campaign.PlanEntry(entryCampaign, first.ID(), entryRecord, nil, "cross-border-entry-1")
	if err != nil || !shouldDeliver {
		t.Fatalf("planning first contact = %#v, deliver=%v, err=%v", intent, shouldDeliver, err)
	}
	execution, err := campaign.CompleteDelivery(intent, 3, campaign.Ack{ExternalRef: "synthetic-channel-ref"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if execution.Key != intent.Key {
		t.Fatalf("execution key = %q, intent key = %q", execution.Key, intent.Key)
	}
	receipt := campaign.Receipt{
		Key: execution.Key, Arrived: true, Outcome: campaign.OutcomeSuccess,
		Attributes: map[string]string{"read": "no"},
	}
	transition, err := campaign.ApplyReceipt(entryCampaign, execution, receipt)
	if err != nil {
		t.Fatal(err)
	}
	if len(transition.Deliveries) != 0 {
		t.Fatalf("first-contact receipt immediately produced reminder intents: %#v", transition.Deliveries)
	}
	if transition.Execution.Key != receipt.Key || transition.Execution.UID != entryRecord.UID {
		t.Fatalf("receipt transition lost execution identity: %#v", transition.Execution)
	}
	if transition.Event.Name != receiptEvent || transition.Event.Key != execution.Key || transition.Event.UID != execution.UID {
		t.Fatalf("receipt backflow event = %#v, want arrival event linked to the first execution", transition.Event)
	}
	backflowRecord := audience.Record{
		UID: transition.Event.UID, Attributes: transition.Event.Attributes,
		EventAges: map[string]time.Duration{string(transition.Event.Name): 4 * time.Hour},
	}
	for _, testCase := range []struct {
		name   string
		record audience.Record
	}{
		{name: "pre-window", record: audience.Record{UID: backflowRecord.UID, Attributes: backflowRecord.Attributes, EventAges: map[string]time.Duration{string(transition.Event.Name): 3*time.Hour + 59*time.Minute}}},
		{name: "read", record: audience.Record{UID: backflowRecord.UID, Attributes: map[string]string{"read": "yes"}, EventAges: backflowRecord.EventAges}},
	} {
		_, shouldDeliver, err := campaign.PlanEntry(reminderCampaign, next.ID(), testCase.record, nil, campaign.Occurrence("cross-border-"+testCase.name))
		if err != nil || shouldDeliver {
			t.Fatalf("reminder route for %s = deliver %v, err=%v; want no SMS", testCase.name, shouldDeliver, err)
		}
	}
	reminderIntent, shouldDeliver, err := campaign.PlanEntry(reminderCampaign, next.ID(), backflowRecord, nil, "cross-border-reminder-1")
	if err != nil || !shouldDeliver || reminderIntent.UID != execution.UID {
		t.Fatalf("four-hour unread reminder plan = %#v, deliver=%v, err=%v", reminderIntent, shouldDeliver, err)
	}
	if reminderIntent.Key == execution.Key {
		t.Fatal("separate reminder activity reused the first-contact idempotency key")
	}
	replayed, err := campaign.ApplyReceipt(entryCampaign, transition.Execution, receipt)
	if err != nil || !replayed.Noop || len(replayed.Deliveries) != 0 {
		t.Fatalf("replayed receipt transition = %#v, err=%v", replayed, err)
	}
	conflict := receipt
	conflict.Outcome = campaign.OutcomeFailure
	conflict.Arrived = false
	if _, err := campaign.ApplyReceipt(entryCampaign, transition.Execution, conflict); !errors.Is(err, campaign.ErrConflictingReceipt) {
		t.Fatalf("conflicting receipt error = %v", err)
	}
}

func scenarioSource(t *testing.T, id string, category audience.Category, granularity time.Duration) audience.Source {
	t.Helper()
	source, err := audience.NewSource(audience.SourceSpec{
		ID: audience.SourceID(id), Supply: audience.SupplyPull, Category: category, BackflowGranularity: granularity,
	})
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func scenarioAssignment(t *testing.T, id string, shares int, colored bool) experimentation.Definition {
	t.Helper()
	split, err := experimentation.EqualSplit(shares)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := experimentation.NewDefinition(experimentation.DefinitionSpec{
		ID: experimentation.AssignmentID(id), Split: split, Salt: experimentation.Salt(id),
		Algorithm: experimentation.FNV1aV1{}.Version(), Colored: colored,
	})
	if err != nil {
		t.Fatal(err)
	}
	return definition
}

func scenarioBucket(t *testing.T, value int) experimentation.BucketNumber {
	t.Helper()
	bucket, err := experimentation.NewBucketNumber(value)
	if err != nil {
		t.Fatal(err)
	}
	return bucket
}

func scenarioDefinition(t *testing.T, id string, source audience.SourceID, conditions ...audience.Condition) audience.Definition {
	t.Helper()
	definition, err := audience.NewDefinition(audience.DefinitionSpec{
		ID: audience.DefinitionID(id), Source: source, Conditions: conditions,
	})
	if err != nil {
		t.Fatal(err)
	}
	return definition
}

func scenarioGroup(t *testing.T, id string, source audience.SourceID, conditions ...audience.Condition) audience.Group {
	t.Helper()
	return audience.Single(scenarioDefinition(t, id, source, conditions...))
}

func scenarioPushAction(t *testing.T, id string, outcomes ...campaign.Outcome) campaign.Action {
	t.Helper()
	action, err := campaign.NewAction(campaign.ActionSpec{
		ID: campaign.ActionID(id), Direction: campaign.DirectionPush, Outcomes: outcomes,
	})
	if err != nil {
		t.Fatal(err)
	}
	return action
}

func scenarioDisplayAction(t *testing.T, id string) campaign.Action {
	t.Helper()
	action, err := campaign.NewAction(campaign.ActionSpec{
		ID: campaign.ActionID(id), Direction: campaign.DirectionPull, RequiresSlot: true, Required: []string{"content"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return action
}

func scenarioTreatment(t *testing.T, id string, action campaign.Action, slot *campaign.Slot) campaign.Treatment {
	t.Helper()
	parameters := map[string]string{}
	if slot != nil {
		parameters["content"] = "constructed"
	}
	call, err := action.Call(parameters)
	if err != nil {
		t.Fatal(err)
	}
	var slotID campaign.SlotID
	if slot != nil {
		slotID = slot.ID()
	}
	treatment, err := campaign.NewTreatment(campaign.TreatmentSpec{ID: campaign.TreatmentID(id), Slot: slotID, Call: call})
	if err != nil {
		t.Fatal(err)
	}
	return treatment
}

func scenarioCampaign(t *testing.T, id string, priority int, selection audience.Selection, treatments []campaign.Treatment, flow campaign.Flow, tables []campaign.DecisionTable) campaign.Campaign {
	t.Helper()
	built, err := campaign.NewCampaign(campaign.CampaignSpec{
		ID: campaign.CampaignID(id), Priority: campaign.NewPriority(priority), Audience: selection,
		Treatments: treatments, Flow: flow, Tables: tables,
	})
	if err != nil {
		t.Fatal(err)
	}
	return built
}
