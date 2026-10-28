package scenarios_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/snack8310/gmp/gmp-core/audience"
	"github.com/snack8310/gmp/gmp-core/campaign"
	"github.com/snack8310/gmp/gmp-core/scenarios"
)

// Serial receipt edge: the later treatment is not an entry, a batch run of it
// is refused, and after receipts it reaches exactly the people whose outcome
// hit the edge. Its own audience is a different non-empty set, so enumerating
// that audience cannot be what produced the deliveries.
func TestReceiptSerialReachesOnlyThePeopleOnTheReceipt(t *testing.T) {
	stack := newStack(t, 90)
	built := mustReceiptFlow(t, stack)
	serial := built.Serial

	if serial.Campaign.ID() != "recall-serial" {
		t.Fatalf("campaign = %q", serial.Campaign.ID())
	}
	if serial.Entry != "recall-entry" || serial.Next != "recall-next" {
		t.Fatalf("entry = %q next = %q", serial.Entry, serial.Next)
	}
	assertEdges(t, serial.Campaign, []campaign.EdgeSpec{{
		From: "recall-entry", To: "recall-next",
		Kind: campaign.EdgeOnReceipt, When: campaign.OutcomeSuccess,
	}})

	entries := entrySet(serial.Campaign)
	if !entries["recall-entry"] || entries["recall-next"] || len(entries) != 1 {
		t.Fatalf("entries = %v, want only recall-entry", entries)
	}

	entryWho := statedMembers(t, stack, "stated-recall-new", "tier", "new")
	nextWho := statedMembers(t, stack, "stated-recall-returning", "tier", "returning")
	if len(entryWho) == 0 || len(nextWho) == 0 {
		t.Fatal("an audience is empty, so a delivery asserts nothing")
	}
	if len(overlap(entryWho, nextWho)) != 0 {
		t.Fatal("the stated audiences overlap, so a delivery could belong to either")
	}
	if !sameUIDs(audienceMembers(t, stack, findTreatment(t, serial.Campaign, "recall-entry").Audience()), entryWho) {
		t.Fatal("the entry treatment is not aimed at the new-tier audience")
	}
	if !sameUIDs(audienceMembers(t, stack, findTreatment(t, serial.Campaign, "recall-next").Audience()), nextWho) {
		t.Fatal("the later treatment is not aimed at the returning audience, so rebuilding that audience could coincide with the receipt")
	}

	if _, err := serial.Runner.Run(serial.Campaign, serial.Next, "entry", audience.Live()); !errors.Is(err, campaign.ErrNotAnEntry) {
		t.Fatalf("batch-running %s: %v", serial.Next, err)
	}
	if effects(t, serial.Deliverers, "recall-mail") != 0 {
		t.Fatal("refusing the batch run still delivered the later step")
	}

	report, err := serial.Runner.Run(serial.Campaign, serial.Entry, "entry", audience.Live())
	if err != nil {
		t.Fatalf("running the entry: %v", err)
	}
	if !sameUIDs(executionUIDs(report.Executions), entryWho) {
		t.Fatalf("entry reached %d people, its audience has %d", len(report.Executions), len(entryWho))
	}
	if effects(t, serial.Deliverers, "recall-mail") != 0 {
		t.Fatal("running the entry followed the edge before any receipt")
	}

	// Both receipts say the downstream took the message. The outcome is what
	// selects the edge; arrival alone must not.
	success, failure := splitReceipts(t, serial.Runner, serial.Campaign, report.Executions)
	got := executedUIDs(t, serial.Log, "recall-next", "recall-mail")
	if !sameUIDs(got, success) {
		t.Fatalf("later step reached %d people, %d receipts said success", len(got), len(success))
	}
	if len(overlap(got, failure)) != 0 {
		t.Fatal("a failure receipt also started the later step")
	}
	if len(overlap(got, nextWho)) != 0 {
		t.Fatal("the later step reached someone in its own audience, so it enumerated that definition instead of reading the receipt")
	}
	if !sameUIDs(deliveredUIDs(t, serial.Deliverers["recall-mail"]), got) {
		t.Fatal("the deliverer and the execution log disagree on who the edge reached")
	}
}

// The refusal has to be visible from the scenarios package. A guard that only
// fails inside campaign would let this assembly grow a batch path again.
func TestRunningTheReceiptTargetFromTheScenarioIsRefused(t *testing.T) {
	stack := newStack(t, 90)
	serial := mustReceiptFlow(t, stack).Serial
	before := serial.Log.Size()
	_, err := serial.Runner.Run(serial.Campaign, "recall-next", "entry", audience.Live())
	if !errors.Is(err, campaign.ErrNotAnEntry) {
		t.Fatalf("batch-running recall-next: %v", err)
	}
	if serial.Log.Size() != before {
		t.Fatal("the refused run still recorded an execution")
	}
	if effects(t, serial.Deliverers, "recall-mail") != 0 || effects(t, serial.Deliverers, "recall-push") != 0 {
		t.Fatal("the refused run delivered")
	}
}

// One exit, two receipt edges. Each person is on exactly one later action.
// The later treatments' audiences are different non-empty partitions, and
// neither is who entered.
func TestReceiptBranchSendsEachOutcomeDownItsOwnEdge(t *testing.T) {
	stack := newStack(t, 90)
	branch := mustReceiptFlow(t, stack).Branch

	if branch.Campaign.ID() != "receipt-branch" {
		t.Fatalf("campaign = %q", branch.Campaign.ID())
	}
	if branch.Entry != "branch-entry" || branch.OnSuccess != "branch-on-success" || branch.OnFailure != "branch-on-failure" {
		t.Fatalf("entry = %q success = %q failure = %q", branch.Entry, branch.OnSuccess, branch.OnFailure)
	}
	assertEdges(t, branch.Campaign, []campaign.EdgeSpec{
		{From: "branch-entry", To: "branch-on-success", Kind: campaign.EdgeOnReceipt, When: campaign.OutcomeSuccess},
		{From: "branch-entry", To: "branch-on-failure", Kind: campaign.EdgeOnReceipt, When: campaign.OutcomeFailure},
	})
	entries := entrySet(branch.Campaign)
	if !entries["branch-entry"] || entries["branch-on-success"] || entries["branch-on-failure"] || len(entries) != 1 {
		t.Fatalf("entries = %v, want only branch-entry", entries)
	}

	entered := statedMembers(t, stack, "stated-branch-other", "tier", "other")
	mailWho := statedMembers(t, stack, "stated-branch-new", "tier", "new")
	smsWho := statedMembers(t, stack, "stated-branch-returning", "tier", "returning")
	if len(entered) == 0 || len(mailWho) == 0 || len(smsWho) == 0 {
		t.Fatal("an audience is empty, so a branch asserts nothing")
	}
	if len(overlap(entered, mailWho)) != 0 || len(overlap(entered, smsWho)) != 0 || len(overlap(mailWho, smsWho)) != 0 {
		t.Fatal("the branch audiences are not three disjoint partitions")
	}
	if !sameUIDs(audienceMembers(t, stack, findTreatment(t, branch.Campaign, "branch-on-success").Audience()), mailWho) {
		t.Fatal("the success treatment is not aimed at the new-tier audience")
	}
	if !sameUIDs(audienceMembers(t, stack, findTreatment(t, branch.Campaign, "branch-on-failure").Audience()), smsWho) {
		t.Fatal("the failure treatment is not aimed at the returning audience")
	}

	report, err := branch.Runner.Run(branch.Campaign, branch.Entry, "entry", audience.Live())
	if err != nil {
		t.Fatalf("running the entry: %v", err)
	}
	if !sameUIDs(executionUIDs(report.Executions), entered) {
		t.Fatalf("entry reached %d people, its audience has %d", len(report.Executions), len(entered))
	}
	if effects(t, branch.Deliverers, "branch-mail") != 0 || effects(t, branch.Deliverers, "branch-sms") != 0 {
		t.Fatal("running the entry followed an edge before any receipt")
	}

	success, failure := splitReceipts(t, branch.Runner, branch.Campaign, report.Executions)
	mail := executedUIDs(t, branch.Log, "branch-on-success", "branch-mail")
	sms := executedUIDs(t, branch.Log, "branch-on-failure", "branch-sms")
	if !sameUIDs(mail, success) {
		t.Fatalf("mail reached %d people, %d receipts said success", len(mail), len(success))
	}
	if !sameUIDs(sms, failure) {
		t.Fatalf("sms reached %d people, %d receipts said failure", len(sms), len(failure))
	}
	if len(overlap(mail, sms)) != 0 || len(overlap(mail, failure)) != 0 || len(overlap(sms, success)) != 0 {
		t.Fatal("a person crossed onto the other outcome's action")
	}
	if len(overlap(mail, mailWho)) != 0 || len(overlap(sms, smsWho)) != 0 {
		t.Fatal("a branch reached someone in the later treatment's own audience")
	}
	if !sameUIDs(deliveredUIDs(t, branch.Deliverers["branch-mail"]), mail) || !sameUIDs(deliveredUIDs(t, branch.Deliverers["branch-sms"]), sms) {
		t.Fatal("the deliverers and the execution log disagree")
	}
}

// Two entries, a parallel edge between them, separate audiences. Running one
// does not start the other, and a success receipt does not either.
func TestParallelEntriesRunTheirOwnAudiences(t *testing.T) {
	stack := newStack(t, 90)
	parallel := mustReceiptFlow(t, stack).Parallel

	if parallel.Campaign.ID() != "parallel-entries" {
		t.Fatalf("campaign = %q", parallel.Campaign.ID())
	}
	if parallel.Left != "parallel-left" || parallel.Right != "parallel-right" {
		t.Fatalf("left = %q right = %q", parallel.Left, parallel.Right)
	}
	assertEdges(t, parallel.Campaign, []campaign.EdgeSpec{{
		From: "parallel-left", To: "parallel-right", Kind: campaign.EdgeParallel,
	}})
	entries := entrySet(parallel.Campaign)
	if !entries["parallel-left"] || !entries["parallel-right"] || len(entries) != 2 {
		t.Fatalf("entries = %v, want both treatments", entries)
	}

	leftWho := statedMembers(t, stack, "stated-parallel-new", "tier", "new")
	rightWho := statedMembers(t, stack, "stated-parallel-cart", "added-to-cart", "yes")
	if len(leftWho) == 0 || len(rightWho) == 0 {
		t.Fatal("an entry audience is empty, so its run asserts nothing")
	}
	if len(leftWho) == len(rightWho) {
		t.Fatal("the two entry audiences are the same size, so one audience used for both runs would still match")
	}
	// Someone is in both. If a run skipped people the other treatment had
	// already touched, that person's absence is the only thing the counts
	// would notice.
	if len(overlap(leftWho, rightWho)) == 0 {
		t.Fatal("the two entry audiences do not overlap, so skipping people already touched would still match each count")
	}
	if !sameUIDs(audienceMembers(t, stack, findTreatment(t, parallel.Campaign, "parallel-left").Audience()), leftWho) {
		t.Fatal("the left entry is not the new-tier audience")
	}
	if !sameUIDs(audienceMembers(t, stack, findTreatment(t, parallel.Campaign, "parallel-right").Audience()), rightWho) {
		t.Fatal("the right entry is not the added-to-cart audience")
	}

	leftReport, err := parallel.Runner.Run(parallel.Campaign, parallel.Left, "entry", audience.Live())
	if err != nil {
		t.Fatalf("running the left entry: %v", err)
	}
	if !sameUIDs(executionUIDs(leftReport.Executions), leftWho) {
		t.Fatalf("left entry reached %d people, its audience has %d", len(leftReport.Executions), len(leftWho))
	}
	if effects(t, parallel.Deliverers, "parallel-notice") != 0 {
		t.Fatal("running the left entry started the right one")
	}
	if len(leftReport.Executions) == 0 {
		t.Fatal("the left entry reached nobody")
	}
	if err := parallel.Runner.RecordReceipt(parallel.Campaign, campaign.Receipt{
		Key: leftReport.Executions[0].Key, Arrived: true, Outcome: campaign.OutcomeSuccess,
	}); err != nil {
		t.Fatalf("recording a receipt on a parallel entry: %v", err)
	}
	if effects(t, parallel.Deliverers, "parallel-notice") != 0 {
		t.Fatal("a success receipt on a parallel edge started the other entry")
	}

	rightReport, err := parallel.Runner.Run(parallel.Campaign, parallel.Right, "entry", audience.Live())
	if err != nil {
		t.Fatalf("running the right entry: %v", err)
	}
	if !sameUIDs(executionUIDs(rightReport.Executions), rightWho) {
		t.Fatalf("right entry reached %d people, its audience has %d", len(rightReport.Executions), len(rightWho))
	}
	leftAfter := executedUIDs(t, parallel.Log, "parallel-left", "parallel-benefit")
	if !sameUIDs(leftAfter, leftWho) {
		t.Fatal("running the right entry changed who the left entry had reached")
	}
	for uid := range overlap(leftWho, rightWho) {
		if !leftAfter[uid] || !executionUIDs(rightReport.Executions)[uid] {
			t.Fatalf("%s is in both audiences but did not receive both actions", uid)
		}
	}
}

// Each treatment in the assembly names one action, stated here rather than
// read off as "whatever the treatment happens to hold". A treatment struct
// that grows a second call, or a slice or array of them, no longer expresses one action.
func TestReceiptFlowTreatmentsEachCarryOneAction(t *testing.T) {
	assertSingleCallField(t, reflect.TypeOf(campaign.Treatment{}))
	assertSingleCallField(t, reflect.TypeOf(campaign.TreatmentSpec{}))

	stack := newStack(t, 30)
	who, err := stack.Definition("one-action-who", mustCondition(t, "tier", "new"))
	if err != nil {
		t.Fatalf("building an audience: %v", err)
	}
	if _, err := campaign.NewTreatment(campaign.TreatmentSpec{
		ID: "packed", Who: who,
	}); !errors.Is(err, campaign.ErrMissingTreatmentCall) {
		t.Fatalf("a treatment with no action: %v", err)
	}

	built := mustReceiptFlow(t, stack)
	if built.Missing != "边上的等待时长，以及渠道回执里的「未读」，都不是动作契约的结果；边表达不了，仍由两个活动经回流来源表达，这里不近似" {
		t.Fatalf("Missing = %q, the wait was dropped or rewritten", built.Missing)
	}
	assertActions(t, built.Serial.Campaign, map[campaign.TreatmentID]campaign.ActionID{
		"recall-entry": "recall-push",
		"recall-next":  "recall-mail",
	})
	assertActions(t, built.Branch.Campaign, map[campaign.TreatmentID]campaign.ActionID{
		"branch-entry":      "branch-rcs",
		"branch-on-success": "branch-mail",
		"branch-on-failure": "branch-sms",
	})
	assertActions(t, built.Parallel.Campaign, map[campaign.TreatmentID]campaign.ActionID{
		"parallel-left":  "parallel-benefit",
		"parallel-right": "parallel-notice",
	})
	assertPushTreatmentsHaveNoSlots(t, built.Serial.Campaign, built.Branch.Campaign, built.Parallel.Campaign)
}

func mustReceiptFlow(t *testing.T, stack *scenarios.Stack) *scenarios.ReceiptFlow {
	t.Helper()
	built, err := stack.ReceiptFlow()
	if err != nil {
		t.Fatalf("building the receipt flow: %v", err)
	}
	return built
}

func assertSingleCallField(t *testing.T, typ reflect.Type) {
	t.Helper()
	if typ.Kind() != reflect.Struct {
		t.Fatalf("%s is not a struct", typ)
	}
	call := reflect.TypeOf(campaign.Call{})
	var calls, many int
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		switch {
		case field.Type == call:
			calls++
		case (field.Type.Kind() == reflect.Slice || field.Type.Kind() == reflect.Array) && field.Type.Elem() == call:
			many++
		}
	}
	if calls != 1 || many != 0 {
		t.Fatalf("%s has %d call fields and %d call slices or arrays; one treatment is one action", typ.Name(), calls, many)
	}
}

func assertActions(t *testing.T, built campaign.Campaign, prescribed map[campaign.TreatmentID]campaign.ActionID) {
	t.Helper()
	seen := map[campaign.TreatmentID]bool{}
	for _, treatment := range built.Treatments() {
		want, held := prescribed[treatment.ID()]
		if !held {
			t.Fatalf("treatment %q is not in the prescription", treatment.ID())
		}
		if got := treatment.Call().Action(); got != want {
			t.Fatalf("treatment %q runs %q, the prescription says %q", treatment.ID(), got, want)
		}
		seen[treatment.ID()] = true
	}
	for id := range prescribed {
		if !seen[id] {
			t.Fatalf("the prescription names %q, the campaign does not hold it", id)
		}
	}
}

func assertEdges(t *testing.T, built campaign.Campaign, want []campaign.EdgeSpec) {
	t.Helper()
	flow, ok := built.Flow()
	if !ok {
		t.Fatalf("campaign %q has no flow", built.ID())
	}
	got := flow.Edges()
	if len(got) != len(want) {
		t.Fatalf("campaign %q has %d edges, the prescription lists %d", built.ID(), len(got), len(want))
	}
	for i, edge := range got {
		spec := want[i]
		if edge.From() != spec.From || edge.To() != spec.To || edge.Kind() != spec.Kind || edge.When() != spec.When {
			t.Fatalf("edge %d is %s -%s/%s-> %s, prescription is %s -%s/%s-> %s",
				i, edge.From(), edge.Kind(), edge.When(), edge.To(),
				spec.From, spec.Kind, spec.When, spec.To)
		}
	}
}

func splitReceipts(t *testing.T, runner *campaign.Runner, built campaign.Campaign, executions []campaign.Execution) (success, failure map[audience.UID]bool) {
	t.Helper()
	if len(executions) < 2 {
		t.Fatalf("reached %d people; both outcomes need someone", len(executions))
	}
	success = map[audience.UID]bool{}
	failure = map[audience.UID]bool{}
	for i, execution := range executions {
		outcome := campaign.OutcomeSuccess
		into := success
		if i%2 == 1 {
			outcome = campaign.OutcomeFailure
			into = failure
		}
		into[execution.UID] = true
		if err := runner.RecordReceipt(built, campaign.Receipt{
			Key: execution.Key, Arrived: true, Outcome: outcome,
		}); err != nil {
			t.Fatalf("recording %s for %s: %v", outcome, execution.UID, err)
		}
	}
	if len(success) == 0 || len(failure) == 0 {
		t.Fatal("one outcome was never recorded, so that side asserts nothing")
	}
	return success, failure
}

func executedUIDs(t *testing.T, log *campaign.MemoryLog, id campaign.TreatmentID, action campaign.ActionID) map[audience.UID]bool {
	t.Helper()
	all, err := log.All()
	if err != nil {
		t.Fatalf("reading the log: %v", err)
	}
	out := map[audience.UID]bool{}
	for _, execution := range all {
		if execution.Treatment != id {
			continue
		}
		if execution.Action != action {
			t.Fatalf("%s recorded action %q under %q, want %q", execution.UID, execution.Action, id, action)
		}
		if execution.State == campaign.StateFailed {
			t.Fatalf("%s failed under %q, so this path did not deliver", execution.UID, id)
		}
		if out[execution.UID] {
			t.Fatalf("%s was recorded twice under %q", execution.UID, id)
		}
		out[execution.UID] = true
	}
	return out
}

func executionUIDs(executions []campaign.Execution) map[audience.UID]bool {
	out := make(map[audience.UID]bool, len(executions))
	for _, execution := range executions {
		out[execution.UID] = true
	}
	return out
}

func deliveredUIDs(t *testing.T, deliverer *campaign.MemoryDeliverer) map[audience.UID]bool {
	t.Helper()
	if deliverer == nil {
		t.Fatal("no deliverer")
	}
	out := map[audience.UID]bool{}
	for _, call := range deliverer.Calls() {
		out[call.UID] = true
	}
	if len(out) != deliverer.Effects() {
		t.Fatalf("deliverer reached %d people in %d effects", len(out), deliverer.Effects())
	}
	return out
}

func effects(t *testing.T, deliverers map[campaign.ActionID]*campaign.MemoryDeliverer, action campaign.ActionID) int {
	t.Helper()
	deliverer, held := deliverers[action]
	if !held {
		t.Fatalf("action %q has no deliverer", action)
	}
	return deliverer.Effects()
}

func statedMembers(t *testing.T, stack *scenarios.Stack, id, attribute, value string) map[audience.UID]bool {
	t.Helper()
	definition, err := stack.Definition(id, mustCondition(t, attribute, value))
	if err != nil {
		t.Fatalf("building definition %s: %v", id, err)
	}
	return audienceMembers(t, stack, definition)
}

func mustCondition(t *testing.T, attribute, value string) audience.Condition {
	t.Helper()
	condition, err := audience.Attribute(attribute, value)
	if err != nil {
		t.Fatalf("building condition %s=%s: %v", attribute, value, err)
	}
	return condition
}

func audienceMembers(t *testing.T, stack *scenarios.Stack, definition audience.Definition) map[audience.UID]bool {
	t.Helper()
	members, err := stack.Audiences.Enumerate(definition, audience.Live())
	if err != nil {
		t.Fatalf("enumerating %s: %v", definition.ID(), err)
	}
	out := make(map[audience.UID]bool, len(members))
	for _, uid := range members {
		if out[uid] {
			t.Fatalf("audience %s lists %s twice", definition.ID(), uid)
		}
		out[uid] = true
	}
	return out
}

func findTreatment(t *testing.T, built campaign.Campaign, id campaign.TreatmentID) campaign.Treatment {
	t.Helper()
	for _, treatment := range built.Treatments() {
		if treatment.ID() == id {
			return treatment
		}
	}
	t.Fatalf("campaign %s does not hold %s", built.ID(), id)
	return campaign.Treatment{}
}

func entrySet(built campaign.Campaign) map[campaign.TreatmentID]bool {
	out := map[campaign.TreatmentID]bool{}
	for _, treatment := range built.Entries() {
		out[treatment.ID()] = true
	}
	return out
}

func overlap(left, right map[audience.UID]bool) map[audience.UID]bool {
	out := map[audience.UID]bool{}
	for uid := range left {
		if right[uid] {
			out[uid] = true
		}
	}
	return out
}

func sameUIDs(got, want map[audience.UID]bool) bool {
	if len(got) != len(want) {
		return false
	}
	for uid := range want {
		if !got[uid] {
			return false
		}
	}
	return true
}
