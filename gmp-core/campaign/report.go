package campaign

import "github.com/snack8310/gmp/gmp-core/audience"

// Plan is one delivery intent shown by a rehearsal.
type Plan struct {
	Key        IdempotencyKey
	UID        audience.UID
	Action     ActionID
	Placements []audience.Placement
}

// Report is the application-facing result of one campaign run.
type Report struct {
	Campaign   CampaignID
	Treatment  TreatmentID
	Rehearsal  bool
	Planned    []Plan
	Executions []Execution
}
