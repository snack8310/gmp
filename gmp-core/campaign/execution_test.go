package campaign_test

import (
	"errors"
	"testing"

	"github.com/snack8310/gmp/gmp-core/audience"
	"github.com/snack8310/gmp/gmp-core/campaign"
	"github.com/snack8310/gmp/gmp-core/campaign/deliverytest"
)

const sendSMS = campaign.ActionID("send-sms")

// pushWorld extends the harness with the push direction: an action we call out
// with, a deliverer to call, and a log to record what happened.
type pushWorld struct {
	*world
	runner    *campaign.Runner
	log       *campaign.MemoryLog
	deliverer *campaign.MemoryDeliverer
	action    campaign.Action
	events    *recordingSink
}

// recordingSink keeps every event so that what the platform published can be
// asserted on rather than assumed.
type recordingSink struct{ published []campaign.Event }

func (s *recordingSink) Publish(event campaign.Event) error {
	s.published = append(s.published, event)
	return nil
}

func (s *recordingSink) named(name campaign.EventName) []campaign.Event {
	var out []campaign.Event
	for _, event := range s.published {
		if event.Name == name {
			out = append(out, event)
		}
	}
	return out
}

func newPushWorld(t *testing.T, population int) *pushWorld {
	t.Helper()
	w := newWorld(t, population)
	action, err := campaign.RegisterAction(campaign.ActionSpec{
		ID: sendSMS, Direction: campaign.DirectionPush, Required: []string{"copy"},
	})
	if err != nil {
		t.Fatalf("registering the action: %v", err)
	}
	log := campaign.NewMemoryLog()
	events := &recordingSink{}
	runner, err := campaign.NewRunner(campaign.RunnerSpec{
		Audiences: w.audiences, Log: log, Events: events, MaxAttempts: 3,
	})
	if err != nil {
		t.Fatalf("building the runner: %v", err)
	}
	deliverer := campaign.NewMemoryDeliverer()
	if err := runner.RegisterDeliverer(sendSMS, deliverer); err != nil {
		t.Fatalf("wiring the deliverer: %v", err)
	}
	return &pushWorld{world: w, runner: runner, log: log, deliverer: deliverer, action: action, events: events}
}

func (p *pushWorld) smsTreatment(t *testing.T, id string, who audience.Definition) campaign.Treatment {
	t.Helper()
	call, err := p.action.Call(map[string]string{"copy": "预热提醒 · 加购有礼"})
	if err != nil {
		t.Fatalf("filling in the action: %v", err)
	}
	treatment, err := campaign.NewTreatment(campaign.TreatmentSpec{
		ID: campaign.TreatmentID(id), Slot: bannerID, Who: who, Call: call,
	})
	if err != nil {
		t.Fatalf("building treatment %q: %v", id, err)
	}
	return treatment
}

// armCampaign is a campaign whose treatment is aimed at one share of an
// assignment, so that executions carry a placement worth recording.
func (p *pushWorld) armCampaign(t *testing.T, id string) campaign.Campaign {
	t.Helper()
	assignment := p.assignment("warmup", 4, false)
	arm := p.definition("arm-two", p.condition(audience.Share(assignment, p.share(2))))
	built, err := campaign.NewCampaign(campaign.CampaignSpec{
		ID:         campaign.CampaignID(id),
		Priority:   campaign.NewPriority(10),
		Treatments: []campaign.Treatment{p.smsTreatment(t, "sms", arm)},
	})
	if err != nil {
		t.Fatalf("building campaign %q: %v", id, err)
	}
	return built
}

// The key a delivery carries has to be the same one the record was filed under
// and the same one a receipt comes back with. A break anywhere in that chain
// leaves an execution nobody can ever close.
func TestOneKeyRunsThroughDeliveryRecordAndReceipt(t *testing.T) {
	w := newPushWorld(t, 200)
	built := w.armCampaign(t, "warmup-sms")

	report, err := w.runner.Run(built, "sms", "first-entry", audience.Live())
	if err != nil {
		t.Fatalf("running the treatment: %v", err)
	}
	if len(report.Executions) == 0 {
		t.Fatal("nobody was delivered to, so the chain asserts nothing")
	}

	delivered := map[campaign.IdempotencyKey]bool{}
	for _, request := range w.deliverer.Calls() {
		delivered[request.Key] = true
	}
	for _, execution := range report.Executions {
		if !delivered[execution.Key] {
			t.Fatalf("execution %q was recorded under a key the deliverer never saw", execution.Key)
		}
		stored, found, err := w.log.Lookup(execution.Key)
		if err != nil || !found {
			t.Fatalf("the log has nothing under %q (found=%v err=%v)", execution.Key, found, err)
		}
		if stored.Key != execution.Key {
			t.Fatalf("the log filed %q under %q", execution.Key, stored.Key)
		}
		// The third segment: a receipt arriving with that same key closes it.
		if err := w.runner.RecordReceipt(campaign.Receipt{Key: execution.Key, Arrived: true, ExternalRef: "carrier-ref"}); err != nil {
			t.Fatalf("recording a receipt for %q: %v", execution.Key, err)
		}
		closed, _, err := w.log.Lookup(execution.Key)
		if err != nil {
			t.Fatalf("reading back %q: %v", execution.Key, err)
		}
		if closed.State != campaign.StateArrived {
			t.Fatalf("%q is in state %q after a receipt said it arrived", execution.Key, closed.State)
		}
	}
}

// A receipt carrying a key nothing was delivered under must be refused. Quietly
// dropping it is how an execution ends up hanging with nobody able to say what
// became of it.
func TestAReceiptThatMatchesNothingIsRefused(t *testing.T) {
	w := newPushWorld(t, 20)
	if err := w.runner.RecordReceipt(campaign.Receipt{Key: "a-key-nothing-was-sent-under", Arrived: true}); !errors.Is(err, campaign.ErrNoSuchExecution) {
		t.Fatalf("an unmatched receipt should be refused, got %v", err)
	}
}

// Retrying must reuse the key. A fresh one would make the downstream treat the
// retry as another delivery, and the person receives two of something.
func TestRetryingReusesTheSameKey(t *testing.T) {
	w := newPushWorld(t, 60)
	built := w.armCampaign(t, "retry")

	// Fail the first two attempts for everyone, so every delivery is retried.
	planned, err := w.runner.Run(built, "sms", "entry", audience.Rehearsal())
	if err != nil {
		t.Fatalf("rehearsing to learn the keys: %v", err)
	}
	if len(planned.Planned) == 0 {
		t.Fatal("the rehearsal planned nothing, so retrying asserts nothing")
	}
	for _, plan := range planned.Planned {
		w.deliverer.FailNext(plan.Key, 2)
	}

	report, err := w.runner.Run(built, "sms", "entry", audience.Live())
	if err != nil {
		t.Fatalf("running the treatment: %v", err)
	}
	perKey := map[campaign.IdempotencyKey]int{}
	for _, request := range w.deliverer.Calls() {
		perKey[request.Key]++
	}
	for _, execution := range report.Executions {
		if execution.Attempts < 2 {
			t.Fatalf("%q was delivered in %d attempts; the first two were supposed to fail", execution.Key, execution.Attempts)
		}
		if perKey[execution.Key] != execution.Attempts {
			t.Fatalf("%q was recorded with %d attempts but the deliverer saw %d calls under that key",
				execution.Key, execution.Attempts, perKey[execution.Key])
		}
		if execution.State != campaign.StateDelivered {
			t.Fatalf("%q ended as %q; the third attempt was supposed to succeed", execution.Key, execution.State)
		}
	}
	// One effect per person despite every one of them being tried repeatedly.
	if w.deliverer.Effects() != len(report.Executions) {
		t.Fatalf("%d deliveries produced %d effects", len(report.Executions), w.deliverer.Effects())
	}
}

// Running the same entry again must not deliver again: the key is derived from
// what identifies the entry, so it comes out the same.
func TestRunningTheSameEntryAgainDeliversOnce(t *testing.T) {
	w := newPushWorld(t, 60)
	built := w.armCampaign(t, "repeat")

	first, err := w.runner.Run(built, "sms", "entry", audience.Live())
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	effectsAfterFirst := w.deliverer.Effects()
	if effectsAfterFirst == 0 {
		t.Fatal("the first run delivered nothing, so the repeat asserts nothing")
	}
	if _, err := w.runner.Run(built, "sms", "entry", audience.Live()); err != nil {
		t.Fatalf("second run under the same occurrence: %v", err)
	}
	if w.deliverer.Effects() != effectsAfterFirst {
		t.Fatalf("re-running one entry produced %d effects, expected the original %d",
			w.deliverer.Effects(), effectsAfterFirst)
	}

	// A different entry is a different delivery: the same person satisfying a
	// condition again is a new entry, not a retry.
	if _, err := w.runner.Run(built, "sms", "second-entry", audience.Live()); err != nil {
		t.Fatalf("running a second entry: %v", err)
	}
	if w.deliverer.Effects() != effectsAfterFirst+len(first.Executions) {
		t.Fatalf("a second entry produced %d effects in total, expected %d",
			w.deliverer.Effects(), effectsAfterFirst+len(first.Executions))
	}
}

// Someone the send failed for is still in their arm. Dropping them would
// quietly filter the population, and the ones who fail tend to have something
// in common.
func TestAFailedDeliveryStillCarriesItsPlacement(t *testing.T) {
	w := newPushWorld(t, 60)
	built := w.armCampaign(t, "doomed")

	planned, err := w.runner.Run(built, "sms", "entry", audience.Rehearsal())
	if err != nil {
		t.Fatalf("rehearsing to learn the keys: %v", err)
	}
	for _, plan := range planned.Planned {
		w.deliverer.FailNext(plan.Key, 99)
	}

	report, err := w.runner.Run(built, "sms", "entry", audience.Live())
	if err != nil {
		t.Fatalf("running the treatment: %v", err)
	}
	if len(report.Executions) == 0 {
		t.Fatal("nothing ran, so this asserts nothing")
	}
	for _, execution := range report.Executions {
		if execution.State != campaign.StateFailed {
			t.Fatalf("%q ended as %q, expected every attempt to have failed", execution.Key, execution.State)
		}
		if execution.Treatment == "" {
			t.Fatalf("%q was recorded without a treatment", execution.Key)
		}
		if len(execution.Placements) == 0 {
			t.Fatalf("%q was recorded without a placement, so its result cannot be attributed to an arm", execution.Key)
		}
		if execution.Placements[0].Share != 2 {
			t.Fatalf("%q was placed in share %d, expected the arm it was aimed at", execution.Key, execution.Placements[0].Share)
		}
	}
}

// A rehearsal takes the same path and records nothing.
func TestRehearsalRecordsNoExecution(t *testing.T) {
	w := newPushWorld(t, 100)
	built := w.armCampaign(t, "rehearsed")

	report, err := w.runner.Run(built, "sms", "entry", audience.Rehearsal())
	if err != nil {
		t.Fatalf("rehearsing: %v", err)
	}
	if len(report.Planned) == 0 {
		t.Fatal("the rehearsal planned nothing, so this asserts nothing")
	}
	if len(report.Executions) != 0 {
		t.Fatalf("a rehearsal produced %d executions", len(report.Executions))
	}
	if w.log.Size() != 0 {
		t.Fatalf("a rehearsal wrote %d records", w.log.Size())
	}
	if len(w.deliverer.Calls()) != 0 {
		t.Fatalf("a rehearsal made %d deliveries", len(w.deliverer.Calls()))
	}

	if _, err := w.runner.Run(built, "sms", "entry", audience.Live()); err != nil {
		t.Fatalf("the real run: %v", err)
	}
	if w.log.Size() == 0 {
		t.Fatal("the real run recorded nothing, so the rehearsal assertion proves nothing")
	}
}

// Nothing is waiting at the other end of a pull action: whoever it was for has
// already gone.
func TestAPullActionCannotBeRunInBatch(t *testing.T) {
	w := newPushWorld(t, 20)
	everyone := w.definition("anyone")
	built, err := campaign.NewCampaign(campaign.CampaignSpec{
		ID:         "pull-in-batch",
		Priority:   campaign.NewPriority(10),
		Treatments: []campaign.Treatment{w.treatment("banner", everyone, "a.png")},
	})
	if err != nil {
		t.Fatalf("building the campaign: %v", err)
	}
	if _, err := w.runner.Run(built, "banner", "entry", audience.Live()); !errors.Is(err, campaign.ErrPullCannotBeDeliveredInBatch) {
		t.Fatalf("a pull action should not be deliverable in a batch, got %v", err)
	}
}

func TestRunnerRefusesWhatItCannotResolve(t *testing.T) {
	w := newPushWorld(t, 20)
	built := w.armCampaign(t, "refusals")

	if _, err := w.runner.Run(built, "sms", "", audience.Live()); !errors.Is(err, campaign.ErrMissingOccurrence) {
		t.Fatalf("expected a missing-occurrence refusal, got %v", err)
	}
	var forgotten audience.Mode
	if _, err := w.runner.Run(built, "sms", "entry", forgotten); !errors.Is(err, audience.ErrModeNotSet) {
		t.Fatalf("expected a missing-mode refusal, got %v", err)
	}
	if _, err := w.runner.Run(built, "no-such-treatment", "entry", audience.Live()); !errors.Is(err, campaign.ErrNoSuchTreatment) {
		t.Fatalf("expected an unknown-treatment refusal, got %v", err)
	}
	if _, err := campaign.NewRunner(campaign.RunnerSpec{
		Audiences: w.audiences, Log: campaign.NewMemoryLog(), Events: campaign.DiscardEvents(), MaxAttempts: 0,
	}); !errors.Is(err, campaign.ErrNonPositiveAttempts) {
		t.Fatalf("expected a non-positive-attempts refusal, got %v", err)
	}
}

// An action nobody wired up has nowhere to go, and that is a wiring mistake
// rather than a campaign that simply reaches no one.
func TestRunnerNeedsADelivererForTheAction(t *testing.T) {
	w := newPushWorld(t, 20)
	bare, err := campaign.NewRunner(campaign.RunnerSpec{
		Audiences: w.audiences, Log: campaign.NewMemoryLog(), Events: campaign.DiscardEvents(), MaxAttempts: 3,
	})
	if err != nil {
		t.Fatalf("building a runner with nothing wired: %v", err)
	}
	built := w.armCampaign(t, "unwired")
	if _, err := bare.Run(built, "sms", "entry", audience.Live()); !errors.Is(err, campaign.ErrNoDeliverer) {
		t.Fatalf("expected a missing-deliverer refusal, got %v", err)
	}
}

// The in-memory deliverer is the first implementation to run the shared
// contract. A real downstream is expected to call the same suite rather than
// assert its own idempotency.
func TestMemoryDelivererMeetsTheContract(t *testing.T) {
	deliverytest.Run(t, func() deliverytest.Subject {
		deliverer := campaign.NewMemoryDeliverer()
		return deliverytest.Subject{Deliverer: deliverer, Effects: deliverer.Effects}
	})
}

// forgetfulDeliverer ignores the key, which is what a downstream that claims
// idempotency and is not looks like from here.
type forgetfulDeliverer struct{ effects int }

func (d *forgetfulDeliverer) Deliver(campaign.Request) (campaign.Ack, error) {
	d.effects++
	return campaign.Ack{}, nil
}

// The contract's central assertion has to be able to tell the two apart, or
// running it against a new downstream would mean nothing.
func TestTheContractsCentralAssertionDiscriminates(t *testing.T) {
	forgetful := &forgetfulDeliverer{}
	request := campaign.Request{Key: "one-key", UID: "u-1"}
	for i := 0; i < 2; i++ {
		if _, err := forgetful.Deliver(request); err != nil {
			t.Fatalf("delivering: %v", err)
		}
	}
	if forgetful.effects == 1 {
		t.Fatal("a deliverer ignoring the key produced one effect, so the contract cannot tell it from an idempotent one")
	}

	honest := campaign.NewMemoryDeliverer()
	for i := 0; i < 2; i++ {
		if _, err := honest.Deliver(request); err != nil {
			t.Fatalf("delivering: %v", err)
		}
	}
	if honest.Effects() != 1 {
		t.Fatalf("the idempotent deliverer produced %d effects under one key", honest.Effects())
	}
}
