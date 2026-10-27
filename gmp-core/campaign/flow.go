package campaign

import (
	"errors"
	"fmt"
)

var (
	// ErrMissingFlowID is returned when a flow has no identity.
	ErrMissingFlowID = errors.New("campaign: flow id is empty")
	// ErrUnknownEdgeKind is returned when an edge does not say whether it
	// waits for a receipt or runs in parallel.
	//
	// The zero value must not mean "parallel". An edge whose condition was
	// forgotten would then start the next treatment for the whole audience
	// instead of waiting, and nothing would say the condition was missing.
	ErrUnknownEdgeKind = errors.New("campaign: edge kind is not one of the known kinds")
	// ErrReceiptEdgeMissingOutcome is returned when a receipt edge does not
	// say which result it waits for.
	ErrReceiptEdgeMissingOutcome = errors.New("campaign: a receipt edge must name an outcome")
	// ErrParallelEdgeHasOutcome is returned when a parallel edge also names a
	// receipt outcome. The two meanings cannot be true of one edge.
	ErrParallelEdgeHasOutcome = errors.New("campaign: a parallel edge must not name a receipt outcome")
	// ErrEdgeToSelf is returned when an edge connects a treatment to itself.
	ErrEdgeToSelf = errors.New("campaign: edge connects a treatment to itself")
	// ErrDuplicateEdge is returned when the same edge is listed twice.
	ErrDuplicateEdge = errors.New("campaign: the same edge appears twice")
	// ErrEdgeTreatmentNotInCampaign is returned when an edge names a treatment
	// the campaign does not hold.
	ErrEdgeTreatmentNotInCampaign = errors.New("campaign: edge names a treatment this campaign does not hold")
	// ErrEdgeOutcomeNotDeclared is returned when a receipt edge waits for a
	// result the source action's contract does not name.
	//
	// Leaving it to run time would mean the edge never matches and the next
	// treatment is simply never reached, which looks like a branch that
	// nobody qualified for.
	ErrEdgeOutcomeNotDeclared = errors.New("campaign: edge outcome is not in the source action's contract")
	// ErrReceiptEdgeTargetsPull is returned when a receipt edge points at a
	// pull action.
	//
	// Following that edge would try to deliver something nobody is waiting
	// for. Refusing it while the campaign is built means the receipt is not
	// the moment the mistake shows up.
	ErrReceiptEdgeTargetsPull = errors.New("campaign: a receipt edge cannot target a pull action")
	// ErrEdgeConflict is returned when one treatment both waits on a receipt
	// and is marked as starting in parallel. One of those would send to the
	// person on the receipt, the other to the audience, and both would look
	// intentional.
	ErrEdgeConflict = errors.New("campaign: a treatment cannot both wait on a receipt and be started in parallel")
	// ErrFlowCycle is returned when receipt edges loop.
	//
	// A later receipt would walk back into a treatment that already ran for
	// this person. There is no settled meaning for that, so it is refused
	// rather than discovered by sending again.
	ErrFlowCycle = errors.New("campaign: receipt edges form a cycle")
	// ErrNotAnEntry is returned when a batch run is asked to enumerate a
	// treatment that a receipt edge points at.
	//
	// That enumeration is the audience path an edge replaces. Taking it as
	// well would deliver twice: once to the audience, once to the person on
	// the receipt.
	ErrNotAnEntry = errors.New("campaign: this treatment is reached by a receipt edge, not by enumerating an audience")
	// ErrReceiptOutcomeRequired is returned when a receipt edge leaves a
	// treatment and the receipt names no outcome.
	//
	// Dropping it would take no edge, and the next treatment would quietly
	// reach nobody.
	ErrReceiptOutcomeRequired = errors.New("campaign: a receipt edge leaves this treatment and the receipt named no outcome")
	// ErrReceiptOutcomeNotDeclared is returned when a receipt reports a result
	// the action's contract does not name.
	ErrReceiptOutcomeNotDeclared = errors.New("campaign: receipt outcome is not in this action's contract")
	// ErrCampaignMismatch is returned when a receipt is applied with a
	// campaign other than the one that owns the execution.
	//
	// The flow on the campaign passed in is the flow that runs. Using a
	// different one would follow the wrong edges, or none, without a failed
	// lookup to point at it.
	ErrCampaignMismatch = errors.New("campaign: receipt belongs to a different campaign")
)

// FlowID identifies one campaign flow (活动流程).
type FlowID string

// EdgeKind says whether an edge waits for a receipt or only records that two
// treatments are started together.
type EdgeKind string

const (
	// EdgeOnReceipt waits for one outcome of the source action's contract and
	// then runs the target for the person that receipt is about.
	EdgeOnReceipt EdgeKind = "receipt"
	// EdgeParallel records that the two treatments are entries. It does not
	// wait, and running one does not start the other.
	EdgeParallel EdgeKind = "parallel"
)

// Edge is a directed link (边) from one treatment to another.
//
// A receipt edge is how an in-campaign next step is chosen. A parallel edge
// does not choose a next step; treatments with no receipt edge between them
// are entries either way.
type Edge struct {
	from TreatmentID
	to   TreatmentID
	kind EdgeKind
	when Outcome
}

// From reports the treatment this edge leaves.
func (e Edge) From() TreatmentID { return e.from }

// To reports the treatment this edge points at.
func (e Edge) To() TreatmentID { return e.to }

// Kind reports whether this edge waits for a receipt.
func (e Edge) Kind() EdgeKind { return e.kind }

// When reports the receipt outcome a receipt edge waits for. A parallel edge
// reports the empty outcome.
func (e Edge) When() Outcome { return e.when }

// EdgeSpec is the input to one edge.
type EdgeSpec struct {
	// From is the treatment the edge leaves.
	From TreatmentID
	// To is the treatment the edge points at.
	To TreatmentID
	// Kind is required. There is no default: a forgotten kind must not be
	// read as parallel.
	Kind EdgeKind
	// When is the receipt outcome. Required for a receipt edge and forbidden
	// for a parallel edge.
	When Outcome
}

// Flow is one campaign's flow (活动流程): the object that connects that
// campaign's treatments with edges.
//
// It is not a field of a treatment, and it does not put more than one action
// on a treatment. Nodes are treatments. A separate product-facing English
// name for this object is still unset; Flow is the name this package uses.
type Flow struct {
	id    FlowID
	edges []Edge
}

// FlowSpec is the input to building a flow.
type FlowSpec struct {
	// ID identifies the flow.
	ID FlowID
	// Edges connect treatments. None means every treatment of the campaign is
	// an entry.
	Edges []EdgeSpec
}

type edgeKey struct {
	from TreatmentID
	to   TreatmentID
	kind EdgeKind
	when Outcome
}

// NewFlow builds a flow, refusing edges that do not say what they are.
//
// Whether the endpoints belong to a campaign, and whether a receipt outcome
// is one the source action declares, is checked when the campaign is built:
// a flow does not hold the treatments.
func NewFlow(spec FlowSpec) (Flow, error) {
	if spec.ID == "" {
		return Flow{}, ErrMissingFlowID
	}
	edges := make([]Edge, 0, len(spec.Edges))
	seen := make(map[edgeKey]bool, len(spec.Edges))
	for _, specEdge := range spec.Edges {
		edge, err := newEdge(spec.ID, specEdge)
		if err != nil {
			return Flow{}, err
		}
		key := edgeKey{from: edge.from, to: edge.to, kind: edge.kind, when: edge.when}
		if seen[key] {
			return Flow{}, fmt.Errorf("%w: %q -> %q in flow %q", ErrDuplicateEdge, edge.from, edge.to, spec.ID)
		}
		seen[key] = true
		edges = append(edges, edge)
	}
	return Flow{id: spec.ID, edges: edges}, nil
}

func newEdge(flow FlowID, spec EdgeSpec) (Edge, error) {
	if spec.From == "" || spec.To == "" {
		return Edge{}, fmt.Errorf("%w: flow %q", ErrMissingTreatmentID, flow)
	}
	if spec.From == spec.To {
		return Edge{}, fmt.Errorf("%w: %q in flow %q", ErrEdgeToSelf, spec.From, flow)
	}
	switch spec.Kind {
	case EdgeOnReceipt:
		if spec.When == "" {
			return Edge{}, fmt.Errorf("%w: %q -> %q in flow %q", ErrReceiptEdgeMissingOutcome, spec.From, spec.To, flow)
		}
	case EdgeParallel:
		if spec.When != "" {
			return Edge{}, fmt.Errorf("%w: %q -> %q in flow %q", ErrParallelEdgeHasOutcome, spec.From, spec.To, flow)
		}
	default:
		return Edge{}, fmt.Errorf("%w: flow %q", ErrUnknownEdgeKind, flow)
	}
	return Edge{from: spec.From, to: spec.To, kind: spec.Kind, when: spec.When}, nil
}

// ID reports which flow this is.
func (f Flow) ID() FlowID { return f.id }

// Edges reports the links, in the order they were given.
func (f Flow) Edges() []Edge {
	out := make([]Edge, len(f.edges))
	copy(out, f.edges)
	return out
}

func validateFlow(campaign CampaignID, flow Flow, treatments map[TreatmentID]Treatment) error {
	gated := map[TreatmentID]bool{}
	parallelTarget := map[TreatmentID]bool{}
	for _, edge := range flow.edges {
		source, sourceOK := treatments[edge.from]
		target, targetOK := treatments[edge.to]
		if !sourceOK || !targetOK {
			missing := edge.from
			if sourceOK {
				missing = edge.to
			}
			return fmt.Errorf("%w: %q in campaign %q", ErrEdgeTreatmentNotInCampaign, missing, campaign)
		}
		switch edge.kind {
		case EdgeOnReceipt:
			if !source.call.Declares(edge.when) {
				return fmt.Errorf("%w: %q on %q in campaign %q", ErrEdgeOutcomeNotDeclared, edge.when, edge.from, campaign)
			}
			if target.call.Direction() == DirectionPull {
				return fmt.Errorf("%w: %q in campaign %q", ErrReceiptEdgeTargetsPull, edge.to, campaign)
			}
			gated[edge.to] = true
		case EdgeParallel:
			parallelTarget[edge.to] = true
		default:
			return fmt.Errorf("%w: campaign %q", ErrUnknownEdgeKind, campaign)
		}
	}
	for id := range gated {
		if parallelTarget[id] {
			return fmt.Errorf("%w: %q in campaign %q", ErrEdgeConflict, id, campaign)
		}
	}
	if receiptCycle(flow.edges) {
		return fmt.Errorf("%w: campaign %q", ErrFlowCycle, campaign)
	}
	return nil
}

// receiptCycle reports whether receipt edges can walk back to a treatment
// they already left. Parallel edges are not followed: they do not send.
func receiptCycle(edges []Edge) bool {
	next := map[TreatmentID][]TreatmentID{}
	nodes := map[TreatmentID]bool{}
	for _, edge := range edges {
		if edge.kind != EdgeOnReceipt {
			continue
		}
		next[edge.from] = append(next[edge.from], edge.to)
		nodes[edge.from] = true
		nodes[edge.to] = true
	}
	const (
		unseen   = 0
		visiting = 1
		seen     = 2
	)
	color := map[TreatmentID]int{}
	var visit func(TreatmentID) bool
	visit = func(id TreatmentID) bool {
		switch color[id] {
		case visiting:
			return true
		case seen:
			return false
		}
		color[id] = visiting
		for _, to := range next[id] {
			if visit(to) {
				return true
			}
		}
		color[id] = seen
		return false
	}
	for id := range nodes {
		if visit(id) {
			return true
		}
	}
	return false
}

// Flow reports this campaign's flow, and whether it has one.
func (c Campaign) Flow() (Flow, bool) {
	if c.flow.id == "" {
		return Flow{}, false
	}
	return c.flow, true
}

// Entries reports the treatments a batch run may enumerate.
//
// A treatment is an entry when no receipt edge points at it. With no flow,
// every treatment is an entry, which is what a campaign of independent
// treatments already was. The order is the campaign's treatment order.
func (c Campaign) Entries() []Treatment {
	gated := map[TreatmentID]bool{}
	for _, edge := range c.flow.edges {
		if edge.kind == EdgeOnReceipt {
			gated[edge.to] = true
		}
	}
	out := make([]Treatment, 0, len(c.treatments))
	for _, treatment := range c.treatments {
		if !gated[treatment.id] {
			out = append(out, treatment)
		}
	}
	return out
}

func (c Campaign) isEntry(id TreatmentID) bool {
	for _, treatment := range c.Entries() {
		if treatment.id == id {
			return true
		}
	}
	return false
}

func (c Campaign) treatment(id TreatmentID) (Treatment, bool) {
	for _, treatment := range c.treatments {
		if treatment.id == id {
			return treatment, true
		}
	}
	return Treatment{}, false
}

func (c Campaign) awaitsReceipt(id TreatmentID) bool {
	for _, edge := range c.flow.edges {
		if edge.kind == EdgeOnReceipt && edge.from == id {
			return true
		}
	}
	return false
}

func (c Campaign) receiptOutcome(treatment Treatment, outcome Outcome) error {
	if c.awaitsReceipt(treatment.id) && outcome == "" {
		return fmt.Errorf("%w: treatment %q", ErrReceiptOutcomeRequired, treatment.id)
	}
	if outcome != "" && !treatment.call.Declares(outcome) {
		return fmt.Errorf("%w: %q on %q", ErrReceiptOutcomeNotDeclared, outcome, treatment.call.Action())
	}
	return nil
}
