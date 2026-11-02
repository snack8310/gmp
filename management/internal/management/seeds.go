package management

func campaignSeeds() []Campaign {
	return []Campaign{
		{
			ID: "campaign-branch", Name: "跨境召回",
			Audience: &AudienceSelection{Type: "group", GroupID: "group-recall"},
			Treatments: []Treatment{
				{ID: "treatment-rcs", Name: "RCS 投放项", ActionID: "action-rcs"},
				{ID: "treatment-email", Name: "邮件投放项", ActionID: "action-email"},
				{ID: "treatment-sms", Name: "短信投放项", ActionID: "action-sms"},
			},
			ActivityFlow: &Flow{
				ID:      "flow-recall",
				Entries: []EntryRoute{{GroupID: "group-recall", Kind: "start", TreatmentIDs: []string{"treatment-rcs"}}},
				Edges: []Edge{
					{ID: "edge-success", FromTreatmentID: "treatment-rcs", ToTreatmentID: "treatment-email", ConditionLabel: "成功"},
					{ID: "edge-failed", FromTreatmentID: "treatment-rcs", ToTreatmentID: "treatment-sms", ConditionLabel: "失败"},
				},
			},
		},
		{
			ID: "campaign-single", Name: "新客欢迎",
			Audience:     &AudienceSelection{Type: "group", GroupID: "group-new"},
			Treatments:   []Treatment{{ID: "treatment-welcome", Name: "欢迎短信", ActionID: "action-sms"}},
			ActivityFlow: &Flow{ID: "flow-welcome", Entries: []EntryRoute{{GroupID: "group-new", Kind: "start", TreatmentIDs: []string{"treatment-welcome"}}}},
		},
		{ID: "campaign-empty", Name: "待装配活动", Treatments: []Treatment{}},
		{
			ID: "campaign-no-edges", Name: "预热实验演练",
			Audience: &AudienceSelection{Type: "bundle", BundleID: "bundle-warmup"},
			Treatments: []Treatment{
				{ID: "treatment-a", Name: "A 臂提醒", ActionID: "action-email"},
				{ID: "treatment-b", Name: "B 臂提醒", ActionID: "action-sms"},
			},
			ActivityFlow: &Flow{ID: "flow-no-edges", Entries: []EntryRoute{
				{GroupID: "group-arm-a", Kind: "start", TreatmentIDs: []string{"treatment-a"}},
				{GroupID: "group-arm-b", Kind: "start", TreatmentIDs: []string{"treatment-b"}},
				{GroupID: "group-arm-control", Kind: "control", TreatmentIDs: []string{}},
			}},
		},
	}
}
