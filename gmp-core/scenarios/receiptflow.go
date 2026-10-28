package scenarios

import (
	"fmt"

	"github.com/snack8310/gmp/gmp-core/audience"
	"github.com/snack8310/gmp/gmp-core/campaign"
)

// Action ids for ReceiptFlow. Each treatment runs one of these. They are
// distinct so a delivery counted on one action cannot be mistaken for another.
const (
	RecallPushAction      = campaign.ActionID("recall-push")
	RecallMailAction      = campaign.ActionID("recall-mail")
	BranchRCSAction       = campaign.ActionID("branch-rcs")
	BranchMailAction      = campaign.ActionID("branch-mail")
	BranchSMSAction       = campaign.ActionID("branch-sms")
	ParallelBenefitAction = campaign.ActionID("parallel-benefit")
	ParallelNoticeAction  = campaign.ActionID("parallel-notice")
)

// ReceiptFlow is the in-campaign shapes an operator configures with edges:
// one receipt step, a branch on one exit's outcome, and entries that do not
// wait on each other.
//
// It is kept apart from ArmCampaign, which hangs the arms of one assignment
// and has no edges, and from TwoStep, which is two campaigns joined by
// backflow. Nothing here is a new domain concept. What an edge cannot say is
// recorded on Missing.
type ReceiptFlow struct {
	Serial   ReceiptSerial
	Branch   ReceiptBranch
	Parallel ReceiptParallel
	// Missing records, in the scenario's own terms, what this build does not
	// express. Empty would mean the wait and the channel's "unread" had been
	// given some stand-in.
	Missing string
}

// ReceiptSerial is one campaign: an entry treatment, then one later treatment
// reached only when the entry's receipt says success.
type ReceiptSerial struct {
	Campaign   campaign.Campaign
	Entry      campaign.TreatmentID
	Next       campaign.TreatmentID
	Runner     *campaign.Runner
	Log        *campaign.MemoryLog
	Deliverers map[campaign.ActionID]*campaign.MemoryDeliverer
}

// ReceiptBranch is one campaign: one exit, two receipt edges. Success and
// failure each name a different later treatment.
type ReceiptBranch struct {
	Campaign   campaign.Campaign
	Entry      campaign.TreatmentID
	OnSuccess  campaign.TreatmentID
	OnFailure  campaign.TreatmentID
	Runner     *campaign.Runner
	Log        *campaign.MemoryLog
	Deliverers map[campaign.ActionID]*campaign.MemoryDeliverer
}

// ReceiptParallel is one campaign whose entries have no receipt edge between
// them. A parallel edge is recorded so the line is visible; it does not run
// anything.
type ReceiptParallel struct {
	Campaign   campaign.Campaign
	Left       campaign.TreatmentID
	Right      campaign.TreatmentID
	Runner     *campaign.Runner
	Log        *campaign.MemoryLog
	Deliverers map[campaign.ActionID]*campaign.MemoryDeliverer
}

// ReceiptFlow builds the three shapes on one stack.
//
// Each shape has its own runner. Events are discarded: the next person comes
// off the receipt inside the campaign package, and a backflow audience is not
// part of the path. Wiring one in would let a rebuilt audience look like the edge.
func (s *Stack) ReceiptFlow() (*ReceiptFlow, error) {
	serial, err := s.receiptSerial()
	if err != nil {
		return nil, err
	}
	branch, err := s.receiptBranch()
	if err != nil {
		return nil, err
	}
	parallel, err := s.receiptParallel()
	if err != nil {
		return nil, err
	}
	return &ReceiptFlow{
		Serial:   serial,
		Branch:   branch,
		Parallel: parallel,
		Missing:  "边上的等待时长，以及渠道回执里的「未读」，都不是动作契约的结果；边表达不了，仍由两个活动经回流来源表达，这里不近似",
	}, nil
}

func (s *Stack) receiptSerial() (ReceiptSerial, error) {
	// The later treatment is aimed at a different non-empty partition than
	// the entry. An empty audience would also be the wrong people, but it
	// would look unconfigured. A real partition shows the edge ignoring a
	// definition someone did write down.
	entryWho, err := s.Definition("recall-new", mustAttribute("tier", "new"))
	if err != nil {
		return ReceiptSerial{}, err
	}
	nextWho, err := s.Definition("recall-returning", mustAttribute("tier", "returning"))
	if err != nil {
		return ReceiptSerial{}, err
	}
	entryCall, err := receiptCall(RecallPushAction, "召回提醒")
	if err != nil {
		return ReceiptSerial{}, err
	}
	nextCall, err := receiptCall(RecallMailAction, "召回邮件")
	if err != nil {
		return ReceiptSerial{}, err
	}
	entry, err := newPushTreatment("recall-entry", entryWho, entryCall)
	if err != nil {
		return ReceiptSerial{}, err
	}
	next, err := newPushTreatment("recall-next", nextWho, nextCall)
	if err != nil {
		return ReceiptSerial{}, err
	}
	flow, err := campaign.NewFlow(campaign.FlowSpec{
		ID: "recall-serial-flow",
		Edges: []campaign.EdgeSpec{{
			From: entry.ID(), To: next.ID(),
			Kind: campaign.EdgeOnReceipt, When: campaign.OutcomeSuccess,
		}},
	})
	if err != nil {
		return ReceiptSerial{}, fmt.Errorf("scenarios: building the serial flow: %w", err)
	}
	built, err := campaignWithFlow("recall-serial", flow, entry, next)
	if err != nil {
		return ReceiptSerial{}, err
	}
	runner, log, deliverers, err := s.discardingRunner(RecallPushAction, RecallMailAction)
	if err != nil {
		return ReceiptSerial{}, err
	}
	return ReceiptSerial{
		Campaign: built, Entry: entry.ID(), Next: next.ID(),
		Runner: runner, Log: log, Deliverers: deliverers,
	}, nil
}

func (s *Stack) receiptBranch() (ReceiptBranch, error) {
	// Three partitions of tier. The later two are not the people who enter,
	// so a branch that enumerated its target would hit the wrong group.
	entryWho, err := s.Definition("branch-other", mustAttribute("tier", "other"))
	if err != nil {
		return ReceiptBranch{}, err
	}
	mailWho, err := s.Definition("branch-mail-new", mustAttribute("tier", "new"))
	if err != nil {
		return ReceiptBranch{}, err
	}
	smsWho, err := s.Definition("branch-sms-returning", mustAttribute("tier", "returning"))
	if err != nil {
		return ReceiptBranch{}, err
	}
	entryCall, err := receiptCall(BranchRCSAction, "RCS 通知")
	if err != nil {
		return ReceiptBranch{}, err
	}
	mailCall, err := receiptCall(BranchMailAction, "成功后的邮件")
	if err != nil {
		return ReceiptBranch{}, err
	}
	smsCall, err := receiptCall(BranchSMSAction, "失败后的短信")
	if err != nil {
		return ReceiptBranch{}, err
	}
	entry, err := newPushTreatment("branch-entry", entryWho, entryCall)
	if err != nil {
		return ReceiptBranch{}, err
	}
	mail, err := newPushTreatment("branch-on-success", mailWho, mailCall)
	if err != nil {
		return ReceiptBranch{}, err
	}
	sms, err := newPushTreatment("branch-on-failure", smsWho, smsCall)
	if err != nil {
		return ReceiptBranch{}, err
	}
	flow, err := campaign.NewFlow(campaign.FlowSpec{
		ID: "receipt-branch-flow",
		Edges: []campaign.EdgeSpec{
			{From: entry.ID(), To: mail.ID(), Kind: campaign.EdgeOnReceipt, When: campaign.OutcomeSuccess},
			{From: entry.ID(), To: sms.ID(), Kind: campaign.EdgeOnReceipt, When: campaign.OutcomeFailure},
		},
	})
	if err != nil {
		return ReceiptBranch{}, fmt.Errorf("scenarios: building the branch flow: %w", err)
	}
	built, err := campaignWithFlow("receipt-branch", flow, entry, mail, sms)
	if err != nil {
		return ReceiptBranch{}, err
	}
	runner, log, deliverers, err := s.discardingRunner(BranchRCSAction, BranchMailAction, BranchSMSAction)
	if err != nil {
		return ReceiptBranch{}, err
	}
	return ReceiptBranch{
		Campaign: built, Entry: entry.ID(), OnSuccess: mail.ID(), OnFailure: sms.ID(),
		Runner: runner, Log: log, Deliverers: deliverers,
	}, nil
}

func (s *Stack) receiptParallel() (ReceiptParallel, error) {
	// Different predicates, both non-empty, and not the same size on the
	// fixture population. Same audience on both would let one run stand in
	// for the other.
	leftWho, err := s.Definition("parallel-new", mustAttribute("tier", "new"))
	if err != nil {
		return ReceiptParallel{}, err
	}
	rightWho, err := s.Definition("parallel-cart", mustAttribute("added-to-cart", "yes"))
	if err != nil {
		return ReceiptParallel{}, err
	}
	leftCall, err := receiptCall(ParallelBenefitAction, "并行权益")
	if err != nil {
		return ReceiptParallel{}, err
	}
	rightCall, err := receiptCall(ParallelNoticeAction, "并行通知")
	if err != nil {
		return ReceiptParallel{}, err
	}
	left, err := newPushTreatment("parallel-left", leftWho, leftCall)
	if err != nil {
		return ReceiptParallel{}, err
	}
	right, err := newPushTreatment("parallel-right", rightWho, rightCall)
	if err != nil {
		return ReceiptParallel{}, err
	}
	flow, err := campaign.NewFlow(campaign.FlowSpec{
		ID: "parallel-entries-flow",
		Edges: []campaign.EdgeSpec{{
			From: left.ID(), To: right.ID(), Kind: campaign.EdgeParallel,
		}},
	})
	if err != nil {
		return ReceiptParallel{}, fmt.Errorf("scenarios: building the parallel flow: %w", err)
	}
	built, err := campaignWithFlow("parallel-entries", flow, left, right)
	if err != nil {
		return ReceiptParallel{}, err
	}
	runner, log, deliverers, err := s.discardingRunner(ParallelBenefitAction, ParallelNoticeAction)
	if err != nil {
		return ReceiptParallel{}, err
	}
	return ReceiptParallel{
		Campaign: built, Left: left.ID(), Right: right.ID(),
		Runner: runner, Log: log, Deliverers: deliverers,
	}, nil
}

// receiptCall registers a push action that admits success and failure. An
// edge can only name a result the contract lists, and a contract that names
// one side must name both.
func receiptCall(id campaign.ActionID, text string) (campaign.Call, error) {
	action, err := campaign.RegisterAction(campaign.ActionSpec{
		ID:        id,
		Direction: campaign.DirectionPush,
		Required:  []string{"copy"},
		Outcomes:  []campaign.Outcome{campaign.OutcomeSuccess, campaign.OutcomeFailure},
	})
	if err != nil {
		return campaign.Call{}, fmt.Errorf("scenarios: registering %q: %w", id, err)
	}
	call, err := action.Call(map[string]string{"copy": text})
	if err != nil {
		return campaign.Call{}, fmt.Errorf("scenarios: filling in %q: %w", id, err)
	}
	return call, nil
}

func newPushTreatment(id campaign.TreatmentID, who audience.Definition, call campaign.Call) (campaign.Treatment, error) {
	treatment, err := campaign.NewTreatment(campaign.TreatmentSpec{
		ID: id, Who: audience.Single(who), Call: call,
	})
	if err != nil {
		return campaign.Treatment{}, fmt.Errorf("scenarios: building treatment %q: %w", id, err)
	}
	return treatment, nil
}

func campaignWithFlow(id campaign.CampaignID, flow campaign.Flow, treatments ...campaign.Treatment) (campaign.Campaign, error) {
	built, err := campaign.NewCampaign(campaign.CampaignSpec{
		ID:         id,
		Priority:   campaign.NewPriority(40),
		Treatments: treatments,
		Flow:       flow,
	})
	if err != nil {
		return campaign.Campaign{}, fmt.Errorf("scenarios: building campaign %q: %w", id, err)
	}
	return built, nil
}

// discardingRunner builds a runner that does not publish into a backflow source.
func (s *Stack) discardingRunner(actions ...campaign.ActionID) (*campaign.Runner, *campaign.MemoryLog, map[campaign.ActionID]*campaign.MemoryDeliverer, error) {
	log := campaign.NewMemoryLog()
	runner, err := campaign.NewRunner(campaign.RunnerSpec{
		Audiences:   s.Audiences,
		Log:         log,
		Events:      campaign.DiscardEvents(),
		MaxAttempts: 3,
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("scenarios: building the runner: %w", err)
	}
	deliverers := make(map[campaign.ActionID]*campaign.MemoryDeliverer, len(actions))
	for _, action := range actions {
		deliverer := campaign.NewMemoryDeliverer()
		if err := runner.RegisterDeliverer(action, deliverer); err != nil {
			return nil, nil, nil, fmt.Errorf("scenarios: wiring %q: %w", action, err)
		}
		deliverers[action] = deliverer
	}
	return runner, log, deliverers, nil
}
