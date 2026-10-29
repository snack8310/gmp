package management

func campaignSeeds() []Campaign {
	groupNew := AudienceRef{Type: "group", GroupID: "group-new"}
	groupRecall := AudienceRef{Type: "group", GroupID: "group-recall"}
	armA := AudienceRef{Type: "arm", BundleID: "bundle-warmup", GroupID: "group-arm-a"}
	armB := AudienceRef{Type: "arm", BundleID: "bundle-warmup", GroupID: "group-arm-b"}
	return []Campaign{
		{
			ID: "campaign-branch", Name: "跨境召回",
			Treatments: []Treatment{
				{ID: "treatment-rcs", Name: "RCS 投放项", ActionID: "action-rcs", AudienceRef: groupRecall},
				{ID: "treatment-email", Name: "邮件投放项", ActionID: "action-email", AudienceRef: armA},
				{ID: "treatment-sms", Name: "短信投放项", ActionID: "action-sms", AudienceRef: armB},
			},
			ActivityFlow: &Flow{ID: "flow-recall", Edges: []Edge{
				{ID: "edge-success", FromTreatmentID: "treatment-rcs", ToTreatmentID: "treatment-email", ConditionLabel: "成功"},
				{ID: "edge-failed", FromTreatmentID: "treatment-rcs", ToTreatmentID: "treatment-sms", ConditionLabel: "失败"},
			}},
		},
		{ID: "campaign-single", Name: "新客欢迎", Treatments: []Treatment{
			{ID: "treatment-welcome", Name: "欢迎短信", ActionID: "action-sms", AudienceRef: groupNew},
		}},
		{ID: "campaign-empty", Name: "待装配活动", Treatments: []Treatment{}},
		{ID: "campaign-no-edges", Name: "无边活动示意", Treatments: []Treatment{
			{ID: "treatment-a", Name: "提醒投放项", ActionID: "action-email", AudienceRef: groupNew},
			{ID: "treatment-b", Name: "提醒投放项", ActionID: "action-sms", AudienceRef: groupRecall},
		}, ActivityFlow: &Flow{ID: "flow-no-edges", Edges: []Edge{}}},
	}
}
