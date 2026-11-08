package management

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"sort"
)

func loadStoreState(ctx context.Context, queryer sqlQueryer) (*Store, error) {
	state := newStore(false)
	state.campaigns = make(map[string]Campaign)
	campaignIDs := make(map[uint64]string)

	rows, err := queryer.QueryContext(ctx, "SELECT reference_type, bundle_id, group_id, name, bundle_name, available FROM gmp_management_audience_references ORDER BY reference_pk")
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var ref AudienceReference
		if err := rows.Scan(&ref.Type, &ref.BundleID, &ref.GroupID, &ref.Name, &ref.BundleName, &ref.Available); err != nil {
			_ = rows.Close()
			return nil, err
		}
		state.references = append(state.references, ref)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	rows, err = queryer.QueryContext(ctx, "SELECT campaign_pk, id_value, name, audience_type, audience_group_id, audience_bundle_id FROM gmp_management_campaigns ORDER BY campaign_pk")
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var (
			pk           uint64
			campaign     Campaign
			audienceType sql.NullString
			groupID      sql.NullString
			bundleID     sql.NullString
		)
		if err := rows.Scan(&pk, &campaign.ID, &campaign.Name, &audienceType, &groupID, &bundleID); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if audienceType.Valid {
			campaign.Audience = &AudienceSelection{Type: audienceType.String, GroupID: groupID.String, BundleID: bundleID.String}
		}
		campaign.Treatments = []Treatment{}
		campaignIDs[pk] = campaign.ID
		state.campaigns[campaign.ID] = campaign
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	rows, err = queryer.QueryContext(ctx, "SELECT campaign_pk, id_value, name, action_id, slot_id FROM gmp_management_treatments ORDER BY treatment_pk")
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var (
			pk     uint64
			id     string
			name   string
			action string
			slotID sql.NullString
		)
		if err := rows.Scan(&pk, &id, &name, &action, &slotID); err != nil {
			_ = rows.Close()
			return nil, err
		}
		campaignID, ok := campaignIDs[pk]
		if !ok {
			_ = rows.Close()
			return nil, errors.New("management: treatment references an unknown campaign")
		}
		campaign := state.campaigns[campaignID]
		campaign.Treatments = append(campaign.Treatments, Treatment{ID: id, Name: name, ActionID: action, SlotID: slotID.String})
		state.campaigns[campaignID] = campaign
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	rows, err = queryer.QueryContext(ctx, "SELECT campaign_pk, flow_id FROM gmp_management_flows ORDER BY campaign_pk")
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var pk uint64
		var flowID string
		if err := rows.Scan(&pk, &flowID); err != nil {
			_ = rows.Close()
			return nil, err
		}
		campaignID, ok := campaignIDs[pk]
		if !ok {
			_ = rows.Close()
			return nil, errors.New("management: flow references an unknown campaign")
		}
		campaign := state.campaigns[campaignID]
		campaign.ActivityFlow = &Flow{ID: flowID, Entries: []EntryRoute{}, Edges: []Edge{}}
		state.campaigns[campaignID] = campaign
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	entryPKs := make(map[uint64]entryLocation)
	rows, err = queryer.QueryContext(ctx, "SELECT entry_pk, campaign_pk, group_id, kind FROM gmp_management_flow_entries ORDER BY campaign_pk, entry_order")
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var (
			entryPK    uint64
			campaignPK uint64
			entry      EntryRoute
		)
		if err := rows.Scan(&entryPK, &campaignPK, &entry.GroupID, &entry.Kind); err != nil {
			_ = rows.Close()
			return nil, err
		}
		campaignID, ok := campaignIDs[campaignPK]
		if !ok {
			_ = rows.Close()
			return nil, errors.New("management: flow entry references an unknown campaign")
		}
		campaign := state.campaigns[campaignID]
		if campaign.ActivityFlow == nil {
			_ = rows.Close()
			return nil, errors.New("management: flow entry has no parent flow")
		}
		entry.TreatmentIDs = []string{}
		campaign.ActivityFlow.Entries = append(campaign.ActivityFlow.Entries, entry)
		state.campaigns[campaignID] = campaign
		entryPKs[entryPK] = entryLocation{campaignID: campaignID, entryIndex: len(campaign.ActivityFlow.Entries) - 1}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	rows, err = queryer.QueryContext(ctx, "SELECT entry_pk, treatment_id FROM gmp_management_flow_entry_treatments ORDER BY entry_pk, treatment_order")
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var entryPK uint64
		var treatmentID string
		if err := rows.Scan(&entryPK, &treatmentID); err != nil {
			_ = rows.Close()
			return nil, err
		}
		location, ok := entryPKs[entryPK]
		if !ok {
			_ = rows.Close()
			return nil, errors.New("management: entry treatment references an unknown entry")
		}
		campaign := state.campaigns[location.campaignID]
		entry := campaign.ActivityFlow.Entries[location.entryIndex]
		entry.TreatmentIDs = append(entry.TreatmentIDs, treatmentID)
		campaign.ActivityFlow.Entries[location.entryIndex] = entry
		state.campaigns[location.campaignID] = campaign
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	rows, err = queryer.QueryContext(ctx, "SELECT campaign_pk, id_value, from_treatment_id, to_treatment_id, condition_label FROM gmp_management_flow_edges ORDER BY campaign_pk, edge_order")
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var campaignPK uint64
		var edge Edge
		if err := rows.Scan(&campaignPK, &edge.ID, &edge.FromTreatmentID, &edge.ToTreatmentID, &edge.ConditionLabel); err != nil {
			_ = rows.Close()
			return nil, err
		}
		campaignID, ok := campaignIDs[campaignPK]
		if !ok {
			_ = rows.Close()
			return nil, errors.New("management: flow edge references an unknown campaign")
		}
		campaign := state.campaigns[campaignID]
		if campaign.ActivityFlow == nil {
			_ = rows.Close()
			return nil, errors.New("management: flow edge has no parent flow")
		}
		campaign.ActivityFlow.Edges = append(campaign.ActivityFlow.Edges, edge)
		state.campaigns[campaignID] = campaign
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	if err := queryer.QueryRowContext(ctx, "SELECT next_value FROM gmp_management_sequences WHERE sequence_name = 'campaign'").Scan(&state.nextCampaign); err != nil {
		return nil, err
	}
	if err := queryer.QueryRowContext(ctx, "SELECT next_value FROM gmp_management_sequences WHERE sequence_name = 'treatment'").Scan(&state.nextTreatment); err != nil {
		return nil, err
	}
	for _, campaign := range state.campaigns {
		if p := state.validate(campaign); p != nil {
			return nil, fmt.Errorf("management: stored campaign failed validation: %s", p.Code)
		}
	}
	return state, nil
}

type entryLocation struct {
	campaignID string
	entryIndex int
}

func saveStoreState(ctx context.Context, tx *sql.Tx, state *Store) error {
	for _, table := range []string{
		"gmp_management_flow_entry_treatments",
		"gmp_management_flow_entries",
		"gmp_management_flow_edges",
		"gmp_management_flows",
		"gmp_management_treatments",
		"gmp_management_campaigns",
		"gmp_management_audience_references",
	} {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table); err != nil {
			return err
		}
	}

	if err := ensureUniqueHashes(referenceIdentityValues(state.references)); err != nil {
		return err
	}
	for _, ref := range state.references {
		identity := referenceIdentity(ref)
		if _, err := tx.ExecContext(ctx, "INSERT INTO gmp_management_audience_references (identity_hash, reference_type, bundle_id, group_id, name, bundle_name, available) VALUES (?, ?, ?, ?, ?, ?, ?)",
			hashValue(identity), ref.Type, ref.BundleID, ref.GroupID, ref.Name, ref.BundleName, ref.Available); err != nil {
			return err
		}
	}

	campaignIDs := make([]string, 0, len(state.campaigns))
	for id := range state.campaigns {
		campaignIDs = append(campaignIDs, id)
	}
	sort.Strings(campaignIDs)
	if err := ensureUniqueHashes(campaignIDs); err != nil {
		return err
	}
	campaignPKs := make(map[string]uint64, len(campaignIDs))
	for _, id := range campaignIDs {
		campaign := state.campaigns[id]
		var audienceType, groupID, bundleID any
		if campaign.Audience != nil {
			audienceType = campaign.Audience.Type
			groupID = campaign.Audience.GroupID
			bundleID = campaign.Audience.BundleID
		}
		result, err := tx.ExecContext(ctx, "INSERT INTO gmp_management_campaigns (id_hash, id_value, name, audience_type, audience_group_id, audience_bundle_id) VALUES (?, ?, ?, ?, ?, ?)",
			hashValue(id), id, campaign.Name, audienceType, groupID, bundleID)
		if err != nil {
			return err
		}
		pk, err := result.LastInsertId()
		if err != nil {
			return err
		}
		campaignPKs[id] = uint64(pk)
	}

	for _, campaignID := range campaignIDs {
		campaign := state.campaigns[campaignID]
		campaignPK := campaignPKs[campaignID]
		treatmentIDs := make([]string, 0, len(campaign.Treatments))
		for _, treatment := range campaign.Treatments {
			treatmentIDs = append(treatmentIDs, treatment.ID)
		}
		if err := ensureUniqueHashes(treatmentIDs); err != nil {
			return err
		}
		for _, treatment := range campaign.Treatments {
			var slotID any
			if treatment.SlotID != "" {
				slotID = treatment.SlotID
			}
			if _, err := tx.ExecContext(ctx, "INSERT INTO gmp_management_treatments (campaign_pk, id_hash, id_value, name, action_id, slot_id) VALUES (?, ?, ?, ?, ?, ?)",
				campaignPK, hashValue(treatment.ID), treatment.ID, treatment.Name, treatment.ActionID, slotID); err != nil {
				return err
			}
		}
		if campaign.ActivityFlow == nil {
			continue
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO gmp_management_flows (campaign_pk, flow_id) VALUES (?, ?)", campaignPK, campaign.ActivityFlow.ID); err != nil {
			return err
		}
		for order, entry := range campaign.ActivityFlow.Entries {
			result, err := tx.ExecContext(ctx, "INSERT INTO gmp_management_flow_entries (campaign_pk, entry_order, group_id, kind) VALUES (?, ?, ?, ?)",
				campaignPK, order, entry.GroupID, entry.Kind)
			if err != nil {
				return err
			}
			entryPK, err := result.LastInsertId()
			if err != nil {
				return err
			}
			for treatmentOrder, treatmentID := range entry.TreatmentIDs {
				if _, err := tx.ExecContext(ctx, "INSERT INTO gmp_management_flow_entry_treatments (entry_pk, treatment_order, treatment_id) VALUES (?, ?, ?)",
					entryPK, treatmentOrder, treatmentID); err != nil {
					return err
				}
			}
		}
		for order, edge := range campaign.ActivityFlow.Edges {
			if _, err := tx.ExecContext(ctx, "INSERT INTO gmp_management_flow_edges (campaign_pk, edge_order, id_value, from_treatment_id, to_treatment_id, condition_label) VALUES (?, ?, ?, ?, ?, ?)",
				campaignPK, order, edge.ID, edge.FromTreatmentID, edge.ToTreatmentID, edge.ConditionLabel); err != nil {
				return err
			}
		}
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO gmp_management_sequences (sequence_name, next_value) VALUES ('campaign', ?), ('treatment', ?) ON DUPLICATE KEY UPDATE next_value = VALUES(next_value)",
		state.nextCampaign, state.nextTreatment); err != nil {
		return err
	}
	return nil
}

func hashValue(value string) []byte {
	hash := sha256.Sum256([]byte(value))
	return hash[:]
}

func ensureUniqueHashes(values []string) error {
	seen := make(map[[32]byte]string, len(values))
	for _, value := range values {
		hash := sha256.Sum256([]byte(value))
		if previous, ok := seen[hash]; ok && previous != value {
			return errors.New("management: identifier hash collision")
		}
		seen[hash] = value
	}
	return nil
}

func referenceIdentityValues(refs []AudienceReference) []string {
	values := make([]string, len(refs))
	for index, ref := range refs {
		values[index] = referenceIdentity(ref)
	}
	return values
}

func referenceIdentity(ref AudienceReference) string {
	return fmt.Sprintf("%d:%s%d:%s%d:%s", len(ref.Type), ref.Type, len(ref.BundleID), ref.BundleID, len(ref.GroupID), ref.GroupID)
}
