package scenarios_test

import (
	"errors"
	"testing"
	"time"

	"github.com/snack8310/gmp/gmp-core/audience"
	"github.com/snack8310/gmp/gmp-core/campaign"
	"github.com/snack8310/gmp/gmp-core/scenarios"
)

// Scenario 5 end to end: reach the long-inactive on one channel, and four hours
// later send a message to whoever did not read it.
//
// The two steps never name each other. What connects them is that the first
// one's results flowed back and became something the second one's audience can
// be built over.
func TestScenarioFiveTwoStepsConnectedOnlyByBackflow(t *testing.T) {
	stack := newStack(t, 300)
	chain, err := stack.CrossBorderWinBack()
	if err != nil {
		t.Fatalf("setting up the win-back: %v", err)
	}

	first, err := chain.Runner.Run(chain.First, chain.FirstStep, "entry", audience.Live())
	if err != nil {
		t.Fatalf("running the first step: %v", err)
	}
	if len(first.Executions) == 0 {
		t.Fatal("the first step reached nobody, so the chain asserts nothing")
	}

	// The channel comes back about each one. Half of them were read.
	read := map[audience.UID]bool{}
	for i, execution := range first.Executions {
		wasRead := i%2 == 0
		read[execution.UID] = wasRead
		status := "no"
		if wasRead {
			status = "yes"
		}
		if err := chain.Runner.RecordReceipt(campaign.Receipt{
			Key: execution.Key, Arrived: true, Attributes: map[string]string{"read": status},
		}); err != nil {
			t.Fatalf("recording a receipt: %v", err)
		}
	}

	// Before the four hours are up, nobody qualifies.
	tooSoon, err := chain.Runner.Run(chain.Second, chain.SecondStep, "follow-up", audience.Live())
	if err != nil {
		t.Fatalf("running the second step too soon: %v", err)
	}
	if len(tooSoon.Executions) != 0 {
		t.Fatalf("the second step reached %d people before the window was up", len(tooSoon.Executions))
	}

	chain.Backflow.Advance(4 * time.Hour)

	second, err := chain.Runner.Run(chain.Second, chain.SecondStep, "follow-up", audience.Live())
	if err != nil {
		t.Fatalf("running the second step: %v", err)
	}
	if len(second.Executions) == 0 {
		t.Fatal("the second step reached nobody after the window, so the chain never closed")
	}
	for _, execution := range second.Executions {
		if read[execution.UID] {
			t.Fatalf("%q read the first message and was chased anyway", execution.UID)
		}
	}
	var unread int
	for _, wasRead := range read {
		if !wasRead {
			unread++
		}
	}
	if len(second.Executions) != unread {
		t.Fatalf("the second step reached %d people, %d had not read the first", len(second.Executions), unread)
	}
}

// Scenario 1, as far as it goes: give a coupon to whoever has not ordered, then
// chase them three days later.
//
// The chase is deliberately incomplete -- see the build's own note -- because
// whether the coupon was used is a fact another system holds.
func TestScenarioOneChasesThreeDaysLater(t *testing.T) {
	stack := newStack(t, 300)
	chain, err := stack.FirstOrderConversion()
	if err != nil {
		t.Fatalf("setting up the conversion: %v", err)
	}
	if chain.Missing == "" {
		t.Fatal("this scenario is known to be partly inexpressible and should say so")
	}

	first, err := chain.Runner.Run(chain.First, chain.FirstStep, "entry", audience.Live())
	if err != nil {
		t.Fatalf("running the first step: %v", err)
	}
	if len(first.Executions) == 0 {
		t.Fatal("the first step reached nobody, so the chain asserts nothing")
	}
	granted := map[audience.UID]bool{}
	for _, execution := range first.Executions {
		granted[execution.UID] = true
	}

	tooSoon, err := chain.Runner.Run(chain.Second, chain.SecondStep, "reminder", audience.Live())
	if err != nil {
		t.Fatalf("running the reminder too soon: %v", err)
	}
	if len(tooSoon.Executions) != 0 {
		t.Fatalf("the reminder went out to %d people before three days had passed", len(tooSoon.Executions))
	}

	chain.Backflow.Advance(72 * time.Hour)

	second, err := chain.Runner.Run(chain.Second, chain.SecondStep, "reminder", audience.Live())
	if err != nil {
		t.Fatalf("running the reminder: %v", err)
	}
	if len(second.Executions) != len(first.Executions) {
		t.Fatalf("the reminder reached %d of the %d who got a coupon", len(second.Executions), len(first.Executions))
	}
	for _, execution := range second.Executions {
		if !granted[execution.UID] {
			t.Fatalf("%q was reminded about a coupon they never got", execution.UID)
		}
	}
}

// The second step reacts to what flowed back, not to the treatment that
// produced it. An event from somewhere else entirely reaches it just the same,
// which is what "treatments do not know about each other" means in practice.
func TestTheSecondStepReactsToTheEventNotTheTreatment(t *testing.T) {
	stack := newStack(t, 60)
	chain, err := stack.CrossBorderWinBack()
	if err != nil {
		t.Fatalf("setting up the win-back: %v", err)
	}

	stranger := audience.UID("someone-another-system-messaged")
	if err := chain.Backflow.Publish(campaign.Event{
		Name:       campaign.ArrivedEvent("send-whatsapp"),
		UID:        stranger,
		Key:        "a-key-from-somewhere-else",
		Campaign:   "not-our-campaign",
		Treatment:  "not-our-treatment",
		Action:     "send-whatsapp",
		Attributes: map[string]string{"read": "no"},
	}); err != nil {
		t.Fatalf("publishing an event from elsewhere: %v", err)
	}
	chain.Backflow.Advance(4 * time.Hour)

	report, err := chain.Runner.Run(chain.Second, chain.SecondStep, "follow-up", audience.Live())
	if err != nil {
		t.Fatalf("running the second step: %v", err)
	}
	var reached bool
	for _, execution := range report.Executions {
		if execution.UID == stranger {
			reached = true
		}
	}
	if !reached {
		t.Fatal("the second step ignored an event it did not produce, so the two steps are coupled after all")
	}
}

// A rehearsal must not put anything into the backflow. An event from a
// rehearsal would trigger the next step, and the preview would have started the
// run it was previewing.
func TestARehearsalPublishesNothing(t *testing.T) {
	stack := newStack(t, 200)
	chain, err := stack.CrossBorderWinBack()
	if err != nil {
		t.Fatalf("setting up the win-back: %v", err)
	}

	planned, err := chain.Runner.Run(chain.First, chain.FirstStep, "entry", audience.Rehearsal())
	if err != nil {
		t.Fatalf("rehearsing the first step: %v", err)
	}
	if len(planned.Planned) == 0 {
		t.Fatal("the rehearsal planned nothing, so this asserts nothing")
	}
	if chain.Backflow.Size() != 0 {
		t.Fatalf("a rehearsal put %d people into the backflow", chain.Backflow.Size())
	}

	if _, err := chain.Runner.Run(chain.First, chain.FirstStep, "entry", audience.Live()); err != nil {
		t.Fatalf("the real run: %v", err)
	}
	if chain.Backflow.Size() == 0 {
		t.Fatal("the real run published nothing, so the rehearsal assertion proves nothing")
	}
}

// A first step that failed every attempt must not tell the next step it
// succeeded.
func TestAFailedFirstStepPublishesNoDeliveredEvent(t *testing.T) {
	stack := newStack(t, 120)
	chain, err := stack.CrossBorderWinBack()
	if err != nil {
		t.Fatalf("setting up the win-back: %v", err)
	}

	planned, err := chain.Runner.Run(chain.First, chain.FirstStep, "entry", audience.Rehearsal())
	if err != nil {
		t.Fatalf("rehearsing to learn the keys: %v", err)
	}
	for _, plan := range planned.Planned {
		chain.Deliveries["send-whatsapp"].FailNext(plan.Key, 99)
	}

	report, err := chain.Runner.Run(chain.First, chain.FirstStep, "entry", audience.Live())
	if err != nil {
		t.Fatalf("running the first step: %v", err)
	}
	if len(report.Executions) == 0 {
		t.Fatal("nothing ran, so this asserts nothing")
	}
	for _, execution := range report.Executions {
		if execution.State != campaign.StateFailed {
			t.Fatalf("%q was supposed to fail every attempt, ended as %q", execution.Key, execution.State)
		}
	}
	if chain.Backflow.Size() != 0 {
		t.Fatalf("deliveries that never succeeded put %d people into the backflow", chain.Backflow.Size())
	}
}

// The guard that has been sitting in the audience layer since it was built
// finally has a source to act on: this platform's own results run hour by hour,
// so asking about ten minutes answers "nobody" for everyone.
func TestTheBackflowRefusesAWindowShorterThanItCanSee(t *testing.T) {
	backflow := scenarios.NewBackflow()
	source, err := backflow.Source()
	if err != nil {
		t.Fatalf("describing the source: %v", err)
	}
	if _, err := audience.ElapsedSince(source, "send-whatsapp:arrived", 10*time.Minute); !errors.Is(err, audience.ErrWindowShorterThanGranularity) {
		t.Fatalf("a ten-minute window over hour-level backflow should be refused, got %v", err)
	}
	if _, err := audience.ElapsedSince(source, "send-whatsapp:arrived", 4*time.Hour); err != nil {
		t.Fatalf("a four-hour window should be accepted, got %v", err)
	}
}

// One action's event must not satisfy a condition written for another's.
//
// If every action published under the same name, a step waiting on "the coupon
// went out" would fire for someone who was merely sent a message -- and the
// only sign would be the wrong people being reached.
func TestOneActionsEventDoesNotSatisfyAnothers(t *testing.T) {
	stack := newStack(t, 40)
	backflow, err := stack.WithBackflow()
	if err != nil {
		t.Fatalf("registering the backflow: %v", err)
	}

	somebody := audience.UID("u-00000")
	if err := backflow.Publish(campaign.Event{
		Name: campaign.DeliveredEvent("grant-coupon"), UID: somebody,
		Key: "a-key", Action: "grant-coupon",
	}); err != nil {
		t.Fatalf("publishing a coupon event: %v", err)
	}
	backflow.Advance(4 * time.Hour)

	source, err := backflow.Source()
	if err != nil {
		t.Fatalf("describing the source: %v", err)
	}
	waitingOnCoupon, err := audience.ElapsedSince(source, string(campaign.DeliveredEvent("grant-coupon")), time.Hour)
	if err != nil {
		t.Fatalf("building the coupon condition: %v", err)
	}
	waitingOnSMS, err := audience.ElapsedSince(source, string(campaign.DeliveredEvent("send-sms")), time.Hour)
	if err != nil {
		t.Fatalf("building the sms condition: %v", err)
	}

	forCoupon, err := audience.NewDefinition(audience.DefinitionSpec{
		ID: "after-coupon", Source: scenarios.BackflowSourceID,
		Conditions: []audience.Condition{waitingOnCoupon},
	})
	if err != nil {
		t.Fatalf("building the coupon definition: %v", err)
	}
	forSMS, err := audience.NewDefinition(audience.DefinitionSpec{
		ID: "after-sms", Source: scenarios.BackflowSourceID,
		Conditions: []audience.Condition{waitingOnSMS},
	})
	if err != nil {
		t.Fatalf("building the sms definition: %v", err)
	}

	inCoupon, err := stack.Audiences.Decide(forCoupon, audience.Live(), somebody)
	if err != nil {
		t.Fatalf("deciding on the coupon definition: %v", err)
	}
	if !inCoupon {
		t.Fatal("the coupon event did not satisfy the condition written for it")
	}
	inSMS, err := stack.Audiences.Decide(forSMS, audience.Live(), somebody)
	if err != nil {
		t.Fatalf("deciding on the sms definition: %v", err)
	}
	if inSMS {
		t.Fatal("a coupon event satisfied a condition waiting on a message, so the two actions are indistinguishable")
	}
}
