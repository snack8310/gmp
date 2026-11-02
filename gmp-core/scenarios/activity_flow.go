package scenarios

import (
	"fmt"

	"github.com/snack8310/gmp/gmp-core/audience"
	"github.com/snack8310/gmp/gmp-core/campaign"
)

func singleEntryFlow(id campaign.FlowID, group audience.Group, treatments []campaign.TreatmentID, edges []campaign.EdgeSpec) (campaign.Flow, error) {
	return campaign.NewFlow(campaign.FlowSpec{
		ID: id,
		Entries: []campaign.EntrySpec{{
			Group: group.ID(), Kind: campaign.EntryStart, Treatments: treatments,
		}},
		Edges: edges,
	})
}

func singleTreatmentCampaign(id campaign.CampaignID, priority int, group audience.Group, treatment campaign.Treatment) (campaign.Campaign, error) {
	flow, err := singleEntryFlow(campaign.FlowID(id+"-flow"), group, []campaign.TreatmentID{treatment.ID()}, nil)
	if err != nil {
		return campaign.Campaign{}, fmt.Errorf("scenarios: building flow for campaign %q: %w", id, err)
	}
	built, err := campaign.NewCampaign(campaign.CampaignSpec{
		ID: id, Priority: campaign.NewPriority(priority), Audience: audience.SelectGroup(group),
		Treatments: []campaign.Treatment{treatment}, Flow: flow,
	})
	if err != nil {
		return campaign.Campaign{}, fmt.Errorf("scenarios: building campaign %q: %w", id, err)
	}
	return built, nil
}
