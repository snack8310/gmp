package campaign_test

import (
	"errors"
	"testing"

	"github.com/snack8310/gmp/gmp-core/audience"
	"github.com/snack8310/gmp/gmp-core/campaign"
)

// A receipt edge runs the next treatment for the person on the receipt, then
// the one after that when its own receipt arrives. The later treatments are
// aimed at an audience that matches nobody, so the only way they are reached
// is the edge. No backflow source is registered.
func TestAReceiptEdgeRunsTheNextStepWithoutANewAudience(t *testing.T) {
	w := newWorld(t, 80)
	outcomes := []campaign.Outcome{campaign.OutcomeSuccess, campaign.OutcomeFailure}
	benefitAction := registerPush(t, "grant-benefit", outcomes...)
	whatsappAction := registerPush(t, "send-whatsapp", outcomes...)
	smsAction := registerPush(t, "send-sms", outcomes...)

	arm := w.definition("arm-one", w.condition(audience.Share(w.assignment("flow-serial", 2, false), w.share(1))))
	nobody := w.definition("nobody", w.condition(audience.Attribute("tier", "absent")))
	benefit := treatmentOn(t, benefitAction, "benefit", arm)
	whatsapp := treatmentOn(t, whatsappAction, "whatsapp", nobody)
	sms := treatmentOn(t, smsAction, "sms", nobody)

	flow := mustFlow(t, "serial",
		campaign.EdgeSpec{From: benefit.ID(), To: whatsapp.ID(), Kind: campaign.EdgeOnReceipt, When: campaign.OutcomeSuccess},
		campaign.EdgeSpec{From: whatsapp.ID(), To: sms.ID(), Kind: campaign.EdgeOnReceipt, When: campaign.OutcomeSuccess},
	)
	built := mustCampaign(t, "serial-benefit", flow, benefit, whatsapp, sms)
	entries := entryIDs(built)
	if !entries[benefit.ID()] || entries[whatsapp.ID()] || entries[sms.ID()] {
		t.Fatalf("entries = %v, want only the benefit treatment", entries)
	}

	runner, log, deliverers, events := flowRunner(t, w.audiences, benefitAction, whatsappAction, smsAction)
	if _, err := runner.Run(built, whatsapp.ID(), "entry", audience.Live()); !errors.Is(err, campaign.ErrNotAnEntry) {
		t.Fatalf("batch-running a receipt target should be refused, got %v", err)
	}
	if deliverers[whatsappAction.ID()].Effects() != 0 {
		t.Fatal("refusing the batch run still delivered the whatsapp step")
	}

	report, err := runner.Run(built, benefit.ID(), "entry", audience.Live())
	if err != nil {
		t.Fatalf("running the entry: %v", err)
	}
	if len(report.Executions) == 0 {
		t.Fatal("the entry reached nobody, so the edge asserts nothing")
	}
	if deliverers[whatsappAction.ID()].Effects() != 0 || deliverers[smsAction.ID()].Effects() != 0 {
		t.Fatal("running the entry followed an edge before any receipt")
	}

	matched, err := w.audiences.Enumerate(nobody, audience.Live())
	if err != nil {
		t.Fatalf("enumerating the target audience: %v", err)
	}
	if len(matched) != 0 {
		t.Fatalf("the target audience matches %d people, so a delivery would not prove the edge", len(matched))
	}

	chosen := report.Executions[0]
	if err := runner.RecordReceipt(built, campaign.Receipt{
		Key: chosen.Key, Arrived: true, Outcome: campaign.OutcomeSuccess,
	}); err != nil {
		t.Fatalf("recording the benefit receipt: %v", err)
	}
	if deliverers[whatsappAction.ID()].Effects() != 1 {
		t.Fatalf("the success edge delivered whatsapp %d times", deliverers[whatsappAction.ID()].Effects())
	}
	if deliverers[smsAction.ID()].Effects() != 0 {
		t.Fatal("the first receipt also started the step after the next one")
	}
	if got := deliverers[whatsappAction.ID()].Calls(); len(got) != 1 || got[0].UID != chosen.UID {
		t.Fatalf("whatsapp went to %v, want only %s", uidsOf(got), chosen.UID)
	}
	second := executionFor(t, log, whatsapp.ID(), chosen.UID)
	if second.Key == chosen.Key {
		t.Fatal("the next step reused the upstream execution key")
	}
	if len(second.Placements) == 0 || second.Placements[0] != chosen.Placements[0] {
		t.Fatalf("edge execution placements = %v, upstream = %v", second.Placements, chosen.Placements)
	}
	closed, found, err := log.Lookup(chosen.Key)
	if err != nil || !found || closed.Outcome != campaign.OutcomeSuccess {
		t.Fatalf("upstream receipt outcome = %q (found=%v err=%v)", closed.Outcome, found, err)
	}
	var sawOutcome bool
	for _, event := range events.named(campaign.ArrivedEvent(benefitAction.ID())) {
		if event.UID == chosen.UID && event.Outcome == campaign.OutcomeSuccess {
			sawOutcome = true
		}
	}
	if !sawOutcome {
		t.Fatal("the arrived event did not carry the contract outcome")
	}

	callsBefore := len(deliverers[whatsappAction.ID()].Calls())
	if err := runner.RecordReceipt(built, campaign.Receipt{
		Key: chosen.Key, Arrived: true, Outcome: campaign.OutcomeSuccess,
	}); err != nil {
		t.Fatalf("recording the same receipt again: %v", err)
	}
	if len(deliverers[whatsappAction.ID()].Calls()) != callsBefore || deliverers[whatsappAction.ID()].Effects() != 1 {
		t.Fatal("recording the same receipt delivered the next step again")
	}

	if err := runner.RecordReceipt(built, campaign.Receipt{
		Key: second.Key, Arrived: true, Outcome: campaign.OutcomeSuccess,
	}); err != nil {
		t.Fatalf("recording the whatsapp receipt: %v", err)
	}
	if deliverers[smsAction.ID()].Effects() != 1 {
		t.Fatalf("the second edge delivered sms %d times", deliverers[smsAction.ID()].Effects())
	}
	if got := deliverers[smsAction.ID()].Calls(); len(got) != 1 || got[0].UID != chosen.UID {
		t.Fatalf("sms went to %v, want only %s", uidsOf(got), chosen.UID)
	}
}

// RCS success goes to mail and failure goes to SMS. Each person is on exactly
// one of those two edges. The later treatments match nobody by audience.
func TestReceiptBranchSendsMailOnSuccessAndSMSOnFailure(t *testing.T) {
	w := newWorld(t, 4)
	outcomes := []campaign.Outcome{campaign.OutcomeSuccess, campaign.OutcomeFailure}
	rcsAction := registerPush(t, "send-rcs", outcomes...)
	mailAction := registerPush(t, "send-mail", outcomes...)
	smsAction := registerPush(t, "send-sms", outcomes...)

	everyone := w.definition("everyone")
	nobody := w.definition("nobody-branch", w.condition(audience.Attribute("tier", "absent")))
	rcs := treatmentOn(t, rcsAction, "rcs", everyone)
	mail := treatmentOn(t, mailAction, "mail", nobody)
	sms := treatmentOn(t, smsAction, "sms-branch", nobody)
	flow := mustFlow(t, "rcs-branch",
		campaign.EdgeSpec{From: rcs.ID(), To: mail.ID(), Kind: campaign.EdgeOnReceipt, When: campaign.OutcomeSuccess},
		campaign.EdgeSpec{From: rcs.ID(), To: sms.ID(), Kind: campaign.EdgeOnReceipt, When: campaign.OutcomeFailure},
	)
	built := mustCampaign(t, "rcs-then-mail-or-sms", flow, rcs, mail, sms)
	runner, _, deliverers, _ := flowRunner(t, w.audiences, rcsAction, mailAction, smsAction)

	report, err := runner.Run(built, rcs.ID(), "entry", audience.Live())
	if err != nil {
		t.Fatalf("running rcs: %v", err)
	}
	if len(report.Executions) < 2 {
		t.Fatalf("rcs reached %d people, the branch needs both outcomes", len(report.Executions))
	}
	want := map[audience.UID]campaign.Outcome{}
	for i, execution := range report.Executions {
		outcome := campaign.OutcomeSuccess
		arrived := true
		if i%2 == 1 {
			outcome = campaign.OutcomeFailure
			arrived = false
		}
		want[execution.UID] = outcome
		if err := runner.RecordReceipt(built, campaign.Receipt{
			Key: execution.Key, Arrived: arrived, Outcome: outcome,
		}); err != nil {
			t.Fatalf("recording %s for %s: %v", outcome, execution.UID, err)
		}
	}

	mailUIDs := uidsOf(deliverers[mailAction.ID()].Calls())
	smsUIDs := uidsOf(deliverers[smsAction.ID()].Calls())
	if len(mailUIDs) == 0 || len(smsUIDs) == 0 {
		t.Fatalf("mail reached %v and sms reached %v; one side of the branch never fired", mailUIDs, smsUIDs)
	}
	for uid, outcome := range want {
		switch outcome {
		case campaign.OutcomeSuccess:
			if !mailUIDs[uid] || smsUIDs[uid] {
				t.Fatalf("%s succeeded but mail=%v sms=%v", uid, mailUIDs[uid], smsUIDs[uid])
			}
		case campaign.OutcomeFailure:
			if !smsUIDs[uid] || mailUIDs[uid] {
				t.Fatalf("%s failed but mail=%v sms=%v", uid, mailUIDs[uid], smsUIDs[uid])
			}
		}
	}
}

// "Blocked" is a result the action's contract names besides success and
// failure. Arrived stays true so a receipt cannot be classified by transport
// success alone: the success edge must not take this person.
func TestAContractOutcomeBesidesSuccessFollowsItsOwnEdge(t *testing.T) {
	w := newWorld(t, 3)
	blocked := campaign.Outcome("blocked")
	rcsAction := registerPush(t, "send-rcs-rich", campaign.OutcomeSuccess, campaign.OutcomeFailure, blocked)
	mailAction := registerPush(t, "send-mail-rich", campaign.OutcomeSuccess, campaign.OutcomeFailure)
	reviewAction := registerPush(t, "manual-review", campaign.OutcomeSuccess, campaign.OutcomeFailure)

	everyone := w.definition("everyone-rich")
	nobody := w.definition("nobody-rich", w.condition(audience.Attribute("tier", "absent")))
	rcs := treatmentOn(t, rcsAction, "rcs-rich", everyone)
	mail := treatmentOn(t, mailAction, "mail-rich", nobody)
	review := treatmentOn(t, reviewAction, "review", nobody)
	flow := mustFlow(t, "rich-branch",
		campaign.EdgeSpec{From: rcs.ID(), To: mail.ID(), Kind: campaign.EdgeOnReceipt, When: campaign.OutcomeSuccess},
		campaign.EdgeSpec{From: rcs.ID(), To: review.ID(), Kind: campaign.EdgeOnReceipt, When: blocked},
	)
	built := mustCampaign(t, "rcs-or-review", flow, rcs, mail, review)
	runner, _, deliverers, _ := flowRunner(t, w.audiences, rcsAction, mailAction, reviewAction)
	report, err := runner.Run(built, rcs.ID(), "entry", audience.Live())
	if err != nil {
		t.Fatalf("running the entry: %v", err)
	}
	if len(report.Executions) < 3 {
		t.Fatalf("the entry reached %d people, want one for each outcome", len(report.Executions))
	}
	receipts := []campaign.Receipt{
		{Key: report.Executions[0].Key, Arrived: true, Outcome: campaign.OutcomeSuccess},
		{Key: report.Executions[1].Key, Arrived: true, Outcome: blocked},
		{Key: report.Executions[2].Key, Arrived: false, Outcome: campaign.OutcomeFailure},
	}
	for _, receipt := range receipts {
		if err := runner.RecordReceipt(built, receipt); err != nil {
			t.Fatalf("recording %s: %v", receipt.Outcome, err)
		}
	}
	mailUIDs := uidsOf(deliverers[mailAction.ID()].Calls())
	reviewUIDs := uidsOf(deliverers[reviewAction.ID()].Calls())
	successUID := report.Executions[0].UID
	blockedUID := report.Executions[1].UID
	failedUID := report.Executions[2].UID
	if !mailUIDs[successUID] || reviewUIDs[successUID] {
		t.Fatalf("success uid %s went to mail=%v review=%v", successUID, mailUIDs[successUID], reviewUIDs[successUID])
	}
	if mailUIDs[blockedUID] || !reviewUIDs[blockedUID] {
		t.Fatalf("blocked uid %s went to mail=%v review=%v; arrival was treated as success", blockedUID, mailUIDs[blockedUID], reviewUIDs[blockedUID])
	}
	if mailUIDs[failedUID] || reviewUIDs[failedUID] {
		t.Fatalf("failure uid %s went to mail=%v review=%v; a missing edge still fired", failedUID, mailUIDs[failedUID], reviewUIDs[failedUID])
	}
}

// One success result may name two targets. Both run, for the same person,
// under different keys. That is two deliveries, not one delivery repeated.
func TestOneOutcomeCanStartTwoTreatments(t *testing.T) {
	w := newWorld(t, 2)
	outcomes := []campaign.Outcome{campaign.OutcomeSuccess, campaign.OutcomeFailure}
	rcsAction := registerPush(t, "send-rcs-both", outcomes...)
	mailAction := registerPush(t, "send-mail-both", outcomes...)
	smsAction := registerPush(t, "send-sms-both", outcomes...)
	everyone := w.definition("everyone-both")
	nobody := w.definition("nobody-both", w.condition(audience.Attribute("tier", "absent")))
	rcs := treatmentOn(t, rcsAction, "rcs-both", everyone)
	mail := treatmentOn(t, mailAction, "mail-both", nobody)
	sms := treatmentOn(t, smsAction, "sms-both", nobody)
	flow := mustFlow(t, "both",
		campaign.EdgeSpec{From: rcs.ID(), To: mail.ID(), Kind: campaign.EdgeOnReceipt, When: campaign.OutcomeSuccess},
		campaign.EdgeSpec{From: rcs.ID(), To: sms.ID(), Kind: campaign.EdgeOnReceipt, When: campaign.OutcomeSuccess},
	)
	built := mustCampaign(t, "success-starts-both", flow, rcs, mail, sms)
	runner, _, deliverers, _ := flowRunner(t, w.audiences, rcsAction, mailAction, smsAction)
	report, err := runner.Run(built, rcs.ID(), "entry", audience.Live())
	if err != nil {
		t.Fatalf("running the entry: %v", err)
	}
	if len(report.Executions) == 0 {
		t.Fatal("the entry reached nobody")
	}
	chosen := report.Executions[0]
	if err := runner.RecordReceipt(built, campaign.Receipt{
		Key: chosen.Key, Arrived: true, Outcome: campaign.OutcomeSuccess,
	}); err != nil {
		t.Fatalf("recording the receipt: %v", err)
	}
	for _, action := range []campaign.Action{mailAction, smsAction} {
		calls := deliverers[action.ID()].Calls()
		if deliverers[action.ID()].Effects() != 1 || len(calls) != 1 || calls[0].UID != chosen.UID {
			t.Fatalf("%s delivered %d effects to %v", action.ID(), deliverers[action.ID()].Effects(), uidsOf(calls))
		}
	}
}

// Several treatments and no edge between them are all entries. Running one
// does not wait for a receipt and does not start the other.
func TestEntriesWithNoEdgeRunWithoutWaiting(t *testing.T) {
	w := newWorld(t, 4)
	benefitAction := registerPush(t, "grant-parallel", campaign.OutcomeSuccess, campaign.OutcomeFailure)
	smsAction := registerPush(t, "send-parallel", campaign.OutcomeSuccess, campaign.OutcomeFailure)
	everyone := w.definition("everyone-parallel")
	benefit := treatmentOn(t, benefitAction, "benefit-parallel", everyone)
	sms := treatmentOn(t, smsAction, "sms-parallel", everyone)
	flow := mustFlow(t, "unconnected")
	built := mustCampaign(t, "parallel-entries", flow, benefit, sms)
	entries := entryIDs(built)
	if !entries[benefit.ID()] || !entries[sms.ID()] {
		t.Fatalf("entries = %v, want both treatments", entries)
	}
	runner, _, deliverers, _ := flowRunner(t, w.audiences, benefitAction, smsAction)
	if _, err := runner.Run(built, benefit.ID(), "entry", audience.Live()); err != nil {
		t.Fatalf("running the benefit entry: %v", err)
	}
	if deliverers[smsAction.ID()].Effects() != 0 {
		t.Fatal("running one entry started the other, so the missing edge was treated as a trigger")
	}
	if _, err := runner.Run(built, sms.ID(), "entry", audience.Live()); err != nil {
		t.Fatalf("running the sms entry: %v", err)
	}
	if deliverers[benefitAction.ID()].Effects() == 0 || deliverers[smsAction.ID()].Effects() == 0 {
		t.Fatal("an entry with no incoming edge was not delivered")
	}
	smsEffects := deliverers[smsAction.ID()].Effects()
	live, err := runner.Run(built, benefit.ID(), "second-entry", audience.Live())
	if err != nil {
		t.Fatalf("running a second entry: %v", err)
	}
	if len(live.Executions) == 0 {
		t.Fatal("the second entry reached nobody")
	}
	if err := runner.RecordReceipt(built, campaign.Receipt{
		Key: live.Executions[0].Key, Arrived: true, Outcome: campaign.OutcomeSuccess,
	}); err != nil {
		t.Fatalf("recording a receipt with no outgoing edge: %v", err)
	}
	if deliverers[smsAction.ID()].Effects() != smsEffects {
		t.Fatal("a receipt on one entry started or dropped the other")
	}
}

// A campaign that never grew a flow still runs each of its treatments. The
// flow is optional.
func TestTwoTreatmentsWithNoFlowBothRun(t *testing.T) {
	w := newWorld(t, 4)
	benefitAction := registerPush(t, "grant-noflo", nil...)
	smsAction := registerPush(t, "send-noflo", nil...)
	everyone := w.definition("everyone-noflow")
	benefit := treatmentOn(t, benefitAction, "benefit-noflow", everyone)
	sms := treatmentOn(t, smsAction, "sms-noflow", everyone)
	built := mustCampaign(t, "no-flow", campaign.Flow{}, benefit, sms)
	if _, ok := built.Flow(); ok {
		t.Fatal("a campaign built without a flow reports one")
	}
	if len(built.Entries()) != 2 {
		t.Fatalf("entries = %d, want both treatments", len(built.Entries()))
	}
	runner, _, deliverers, _ := flowRunner(t, w.audiences, benefitAction, smsAction)
	for _, treatment := range built.Entries() {
		if _, err := runner.Run(built, treatment.ID(), "entry", audience.Live()); err != nil {
			t.Fatalf("running entry %s: %v", treatment.ID(), err)
		}
	}
	if deliverers[benefitAction.ID()].Effects() == 0 || deliverers[smsAction.ID()].Effects() == 0 {
		t.Fatal("a treatment on a campaign with no flow was not delivered")
	}
}

// A parallel edge is a recorded line, not a trigger and not a wait.
func TestAParallelEdgeDoesNotWaitOrTrigger(t *testing.T) {
	w := newWorld(t, 4)
	benefitAction := registerPush(t, "grant-together", campaign.OutcomeSuccess, campaign.OutcomeFailure)
	smsAction := registerPush(t, "send-together", campaign.OutcomeSuccess, campaign.OutcomeFailure)
	everyone := w.definition("everyone-together")
	benefit := treatmentOn(t, benefitAction, "benefit-together", everyone)
	sms := treatmentOn(t, smsAction, "sms-together", everyone)
	flow := mustFlow(t, "together", campaign.EdgeSpec{
		From: benefit.ID(), To: sms.ID(), Kind: campaign.EdgeParallel,
	})
	built := mustCampaign(t, "parallel-edge", flow, benefit, sms)
	seen, ok := built.Flow()
	if !ok || len(seen.Edges()) != 1 || seen.Edges()[0].Kind() != campaign.EdgeParallel {
		t.Fatalf("flow edges = %+v, ok=%v", seen.Edges(), ok)
	}
	entries := entryIDs(built)
	if !entries[benefit.ID()] || !entries[sms.ID()] {
		t.Fatalf("a parallel edge gated an entry: %v", entries)
	}
	runner, _, deliverers, _ := flowRunner(t, w.audiences, benefitAction, smsAction)
	report, err := runner.Run(built, benefit.ID(), "entry", audience.Live())
	if err != nil {
		t.Fatalf("running one side: %v", err)
	}
	if deliverers[smsAction.ID()].Effects() != 0 {
		t.Fatal("a parallel edge started the other treatment")
	}
	if len(report.Executions) == 0 {
		t.Fatal("the side that ran reached nobody")
	}
	if _, err := runner.Run(built, sms.ID(), "entry", audience.Live()); err != nil {
		t.Fatalf("running the other side without a receipt: %v", err)
	}
	if deliverers[smsAction.ID()].Effects() == 0 {
		t.Fatal("the other side of a parallel edge did not run on its own")
	}
}

func TestAReceiptEdgeRefusesAReceiptWithNoOutcome(t *testing.T) {
	w := newWorld(t, 2)
	outcomes := []campaign.Outcome{campaign.OutcomeSuccess, campaign.OutcomeFailure}
	rcsAction := registerPush(t, "send-rcs-bare", outcomes...)
	mailAction := registerPush(t, "send-mail-bare", outcomes...)
	everyone := w.definition("everyone-bare")
	rcs := treatmentOn(t, rcsAction, "rcs-bare", everyone)
	mail := treatmentOn(t, mailAction, "mail-bare", everyone)
	flow := mustFlow(t, "bare", campaign.EdgeSpec{
		From: rcs.ID(), To: mail.ID(), Kind: campaign.EdgeOnReceipt, When: campaign.OutcomeSuccess,
	})
	built := mustCampaign(t, "needs-an-outcome", flow, rcs, mail)
	runner, log, deliverers, _ := flowRunner(t, w.audiences, rcsAction, mailAction)
	report, err := runner.Run(built, rcs.ID(), "entry", audience.Live())
	if err != nil {
		t.Fatalf("running the entry: %v", err)
	}
	if len(report.Executions) == 0 {
		t.Fatal("the entry reached nobody")
	}
	key := report.Executions[0].Key
	if err := runner.RecordReceipt(built, campaign.Receipt{Key: key, Arrived: true}); !errors.Is(err, campaign.ErrReceiptOutcomeRequired) {
		t.Fatalf("a receipt with no outcome should be refused, got %v", err)
	}
	if deliverers[mailAction.ID()].Effects() != 0 {
		t.Fatal("refusing the receipt still followed the edge")
	}
	stored, found, err := log.Lookup(key)
	if err != nil || !found {
		t.Fatalf("looking up the execution: found=%v err=%v", found, err)
	}
	if stored.State != campaign.StateDelivered || stored.Outcome != "" {
		t.Fatalf("refused receipt changed the execution to %q / %q", stored.State, stored.Outcome)
	}
}

func TestAReceiptOutcomeOutsideTheContractIsRefused(t *testing.T) {
	w := newWorld(t, 2)
	action := registerPush(t, "send-strict", campaign.OutcomeSuccess, campaign.OutcomeFailure)
	everyone := w.definition("everyone-strict")
	treatment := treatmentOn(t, action, "strict", everyone)
	built := mustCampaign(t, "strict-contract", campaign.Flow{}, treatment)
	runner, log, _, _ := flowRunner(t, w.audiences, action)
	report, err := runner.Run(built, treatment.ID(), "entry", audience.Live())
	if err != nil {
		t.Fatalf("running: %v", err)
	}
	if len(report.Executions) == 0 {
		t.Fatal("reached nobody")
	}
	key := report.Executions[0].Key
	if err := runner.RecordReceipt(built, campaign.Receipt{
		Key: key, Arrived: true, Outcome: campaign.Outcome("blocked"),
	}); !errors.Is(err, campaign.ErrReceiptOutcomeNotDeclared) {
		t.Fatalf("an outcome the contract does not name should be refused, got %v", err)
	}
	stored, _, err := log.Lookup(key)
	if err != nil {
		t.Fatalf("looking up: %v", err)
	}
	if stored.State != campaign.StateDelivered {
		t.Fatalf("refused receipt moved the execution to %q", stored.State)
	}
	if err := runner.RecordReceipt(built, campaign.Receipt{
		Key: key, Arrived: true, Outcome: campaign.OutcomeSuccess,
	}); err != nil {
		t.Fatalf("a declared outcome should be accepted, got %v", err)
	}
	stored, _, err = log.Lookup(key)
	if err != nil {
		t.Fatalf("looking up after the accepted receipt: %v", err)
	}
	if stored.Outcome != campaign.OutcomeSuccess || stored.State != campaign.StateArrived {
		t.Fatalf("accepted receipt left state %q outcome %q", stored.State, stored.Outcome)
	}
}

func TestAReceiptForAnotherCampaignIsRefused(t *testing.T) {
	w := newWorld(t, 2)
	action := registerPush(t, "send-owned", campaign.OutcomeSuccess, campaign.OutcomeFailure)
	everyone := w.definition("everyone-owned")
	treatment := treatmentOn(t, action, "owned", everyone)
	built := mustCampaign(t, "owner", campaign.Flow{}, treatment)
	otherTreatment := treatmentOn(t, action, "other", everyone)
	other := mustCampaign(t, "not-the-owner", campaign.Flow{}, otherTreatment)
	runner, _, deliverers, _ := flowRunner(t, w.audiences, action)
	report, err := runner.Run(built, treatment.ID(), "entry", audience.Live())
	if err != nil {
		t.Fatalf("running: %v", err)
	}
	if len(report.Executions) == 0 {
		t.Fatal("reached nobody")
	}
	err = runner.RecordReceipt(other, campaign.Receipt{
		Key: report.Executions[0].Key, Arrived: true, Outcome: campaign.OutcomeSuccess,
	})
	if !errors.Is(err, campaign.ErrCampaignMismatch) {
		t.Fatalf("a receipt applied to another campaign should be refused, got %v", err)
	}
	if deliverers[action.ID()].Effects() == 0 {
		t.Fatal("the entry delivery itself disappeared")
	}
}

func TestFlowRefusesArrangementsThatFailSilently(t *testing.T) {
	w := newWorld(t, 2)
	outcomes := []campaign.Outcome{campaign.OutcomeSuccess, campaign.OutcomeFailure}
	rcsAction := registerPush(t, "send-rcs-guard", outcomes...)
	mailAction := registerPush(t, "send-mail-guard", outcomes...)
	smsAction := registerPush(t, "send-sms-guard", outcomes...)
	everyone := w.definition("everyone-guard")
	rcs := treatmentOn(t, rcsAction, "rcs", everyone)
	mail := treatmentOn(t, mailAction, "mail", everyone)
	sms := treatmentOn(t, smsAction, "sms", everyone)
	banner := w.treatment("banner", everyone, "a.png")

	t.Run("flow id", func(t *testing.T) {
		_, err := campaign.NewFlow(campaign.FlowSpec{})
		if !errors.Is(err, campaign.ErrMissingFlowID) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("edge kind", func(t *testing.T) {
		_, err := campaign.NewFlow(campaign.FlowSpec{ID: "k", Edges: []campaign.EdgeSpec{{
			From: rcs.ID(), To: mail.ID(),
		}}})
		if !errors.Is(err, campaign.ErrUnknownEdgeKind) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("receipt outcome", func(t *testing.T) {
		_, err := campaign.NewFlow(campaign.FlowSpec{ID: "k", Edges: []campaign.EdgeSpec{{
			From: rcs.ID(), To: mail.ID(), Kind: campaign.EdgeOnReceipt,
		}}})
		if !errors.Is(err, campaign.ErrReceiptEdgeMissingOutcome) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("parallel outcome", func(t *testing.T) {
		_, err := campaign.NewFlow(campaign.FlowSpec{ID: "k", Edges: []campaign.EdgeSpec{{
			From: rcs.ID(), To: mail.ID(), Kind: campaign.EdgeParallel, When: campaign.OutcomeSuccess,
		}}})
		if !errors.Is(err, campaign.ErrParallelEdgeHasOutcome) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("self", func(t *testing.T) {
		_, err := campaign.NewFlow(campaign.FlowSpec{ID: "k", Edges: []campaign.EdgeSpec{{
			From: rcs.ID(), To: rcs.ID(), Kind: campaign.EdgeOnReceipt, When: campaign.OutcomeSuccess,
		}}})
		if !errors.Is(err, campaign.ErrEdgeToSelf) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("duplicate", func(t *testing.T) {
		edge := campaign.EdgeSpec{From: rcs.ID(), To: mail.ID(), Kind: campaign.EdgeOnReceipt, When: campaign.OutcomeSuccess}
		_, err := campaign.NewFlow(campaign.FlowSpec{ID: "k", Edges: []campaign.EdgeSpec{edge, edge}})
		if !errors.Is(err, campaign.ErrDuplicateEdge) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("missing endpoint", func(t *testing.T) {
		_, err := campaign.NewFlow(campaign.FlowSpec{ID: "k", Edges: []campaign.EdgeSpec{{
			From: rcs.ID(), Kind: campaign.EdgeParallel,
		}}})
		if !errors.Is(err, campaign.ErrMissingTreatmentID) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("unknown treatment", func(t *testing.T) {
		flow := mustFlow(t, "missing", campaign.EdgeSpec{
			From: rcs.ID(), To: "not-a-treatment", Kind: campaign.EdgeOnReceipt, When: campaign.OutcomeSuccess,
		})
		_, err := buildCampaign("missing-treatment", flow, rcs, mail)
		if !errors.Is(err, campaign.ErrEdgeTreatmentNotInCampaign) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("undeclared outcome", func(t *testing.T) {
		flow := mustFlow(t, "undeclared", campaign.EdgeSpec{
			From: rcs.ID(), To: mail.ID(), Kind: campaign.EdgeOnReceipt, When: campaign.Outcome("blocked"),
		})
		_, err := buildCampaign("undeclared", flow, rcs, mail)
		if !errors.Is(err, campaign.ErrEdgeOutcomeNotDeclared) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("only success", func(t *testing.T) {
		_, err := campaign.RegisterAction(campaign.ActionSpec{
			ID: "half", Direction: campaign.DirectionPush, Required: []string{"copy"},
			Outcomes: []campaign.Outcome{campaign.OutcomeSuccess},
		})
		if !errors.Is(err, campaign.ErrIncompleteReceiptContract) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("duplicate outcome", func(t *testing.T) {
		_, err := campaign.RegisterAction(campaign.ActionSpec{
			ID: "dup", Direction: campaign.DirectionPush, Required: []string{"copy"},
			Outcomes: []campaign.Outcome{campaign.OutcomeSuccess, campaign.OutcomeFailure, campaign.OutcomeSuccess},
		})
		if !errors.Is(err, campaign.ErrDuplicateOutcome) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("empty outcome", func(t *testing.T) {
		_, err := campaign.RegisterAction(campaign.ActionSpec{
			ID: "blank", Direction: campaign.DirectionPush, Required: []string{"copy"},
			Outcomes: []campaign.Outcome{campaign.OutcomeSuccess, ""},
		})
		if !errors.Is(err, campaign.ErrEmptyOutcome) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("cycle", func(t *testing.T) {
		flow := mustFlow(t, "cycle",
			campaign.EdgeSpec{From: rcs.ID(), To: mail.ID(), Kind: campaign.EdgeOnReceipt, When: campaign.OutcomeSuccess},
			campaign.EdgeSpec{From: mail.ID(), To: rcs.ID(), Kind: campaign.EdgeOnReceipt, When: campaign.OutcomeFailure},
		)
		_, err := buildCampaign("cycle", flow, rcs, mail)
		if !errors.Is(err, campaign.ErrFlowCycle) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("join is not a cycle", func(t *testing.T) {
		flow := mustFlow(t, "join",
			campaign.EdgeSpec{From: rcs.ID(), To: mail.ID(), Kind: campaign.EdgeOnReceipt, When: campaign.OutcomeSuccess},
			campaign.EdgeSpec{From: rcs.ID(), To: sms.ID(), Kind: campaign.EdgeOnReceipt, When: campaign.OutcomeFailure},
			campaign.EdgeSpec{From: mail.ID(), To: sms.ID(), Kind: campaign.EdgeOnReceipt, When: campaign.OutcomeSuccess},
		)
		if _, err := buildCampaign("join", flow, rcs, mail, sms); err != nil {
			t.Fatalf("a join should build, got %v", err)
		}
	})
	t.Run("pull target", func(t *testing.T) {
		flow := mustFlow(t, "pull", campaign.EdgeSpec{
			From: rcs.ID(), To: banner.ID(), Kind: campaign.EdgeOnReceipt, When: campaign.OutcomeSuccess,
		})
		_, err := buildCampaign("pull-target", flow, rcs, banner)
		if !errors.Is(err, campaign.ErrReceiptEdgeTargetsPull) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("parallel and receipt", func(t *testing.T) {
		flow := mustFlow(t, "conflict",
			campaign.EdgeSpec{From: rcs.ID(), To: mail.ID(), Kind: campaign.EdgeOnReceipt, When: campaign.OutcomeSuccess},
			campaign.EdgeSpec{From: sms.ID(), To: mail.ID(), Kind: campaign.EdgeParallel},
		)
		_, err := buildCampaign("conflict", flow, rcs, mail, sms)
		if !errors.Is(err, campaign.ErrEdgeConflict) {
			t.Fatalf("got %v", err)
		}
	})
}

func registerPush(t *testing.T, id string, outcomes ...campaign.Outcome) campaign.Action {
	t.Helper()
	action, err := campaign.RegisterAction(campaign.ActionSpec{
		ID: campaign.ActionID(id), Direction: campaign.DirectionPush,
		Required: []string{"copy"}, Outcomes: outcomes,
	})
	if err != nil {
		t.Fatalf("registering %s: %v", id, err)
	}
	return action
}

func treatmentOn(t *testing.T, action campaign.Action, id string, who audience.Definition) campaign.Treatment {
	t.Helper()
	call, err := action.Call(map[string]string{"copy": "预设文案"})
	if err != nil {
		t.Fatalf("filling in %s: %v", id, err)
	}
	treatment, err := campaign.NewTreatment(campaign.TreatmentSpec{
		ID: campaign.TreatmentID(id), Who: who, Call: call,
	})
	if err != nil {
		t.Fatalf("building treatment %s: %v", id, err)
	}
	return treatment
}

func mustFlow(t *testing.T, id string, edges ...campaign.EdgeSpec) campaign.Flow {
	t.Helper()
	flow, err := campaign.NewFlow(campaign.FlowSpec{ID: campaign.FlowID(id), Edges: edges})
	if err != nil {
		t.Fatalf("building flow %s: %v", id, err)
	}
	return flow
}

func mustCampaign(t *testing.T, id string, flow campaign.Flow, treatments ...campaign.Treatment) campaign.Campaign {
	t.Helper()
	built, err := buildCampaign(id, flow, treatments...)
	if err != nil {
		t.Fatalf("building campaign %s: %v", id, err)
	}
	return built
}

func buildCampaign(id string, flow campaign.Flow, treatments ...campaign.Treatment) (campaign.Campaign, error) {
	spec := campaign.CampaignSpec{
		ID:         campaign.CampaignID(id),
		Priority:   campaign.NewPriority(10),
		Treatments: treatments,
	}
	if flow.ID() != "" {
		spec.Flow = flow
	}
	return campaign.NewCampaign(spec)
}

func flowRunner(t *testing.T, audiences *audience.Service, actions ...campaign.Action) (*campaign.Runner, *campaign.MemoryLog, map[campaign.ActionID]*campaign.MemoryDeliverer, *recordingSink) {
	t.Helper()
	log := campaign.NewMemoryLog()
	events := &recordingSink{}
	runner, err := campaign.NewRunner(campaign.RunnerSpec{
		Audiences: audiences, Log: log, Events: events, MaxAttempts: 3,
	})
	if err != nil {
		t.Fatalf("building the runner: %v", err)
	}
	deliverers := make(map[campaign.ActionID]*campaign.MemoryDeliverer, len(actions))
	for _, action := range actions {
		deliverer := campaign.NewMemoryDeliverer()
		if err := runner.RegisterDeliverer(action.ID(), deliverer); err != nil {
			t.Fatalf("wiring %s: %v", action.ID(), err)
		}
		deliverers[action.ID()] = deliverer
	}
	return runner, log, deliverers, events
}

func entryIDs(c campaign.Campaign) map[campaign.TreatmentID]bool {
	out := map[campaign.TreatmentID]bool{}
	for _, treatment := range c.Entries() {
		out[treatment.ID()] = true
	}
	return out
}

func executionFor(t *testing.T, log *campaign.MemoryLog, id campaign.TreatmentID, uid audience.UID) campaign.Execution {
	t.Helper()
	all, err := log.All()
	if err != nil {
		t.Fatalf("reading the log: %v", err)
	}
	var found campaign.Execution
	n := 0
	for _, execution := range all {
		if execution.Treatment == id && execution.UID == uid {
			found = execution
			n++
		}
	}
	if n != 1 {
		t.Fatalf("log holds %d executions for %s / %s", n, id, uid)
	}
	return found
}

func uidsOf(calls []campaign.Request) map[audience.UID]bool {
	out := map[audience.UID]bool{}
	for _, call := range calls {
		out[call.UID] = true
	}
	return out
}
